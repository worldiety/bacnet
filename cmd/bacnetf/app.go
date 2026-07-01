package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/worldiety/bacnet"
	"github.com/worldiety/bacnet/apdu"
	baclog "github.com/worldiety/bacnet/common/log"
	"github.com/worldiety/bacnet/common/netprim"
)

// commonOptions holds the flags shared by every subcommand.
type commonOptions struct {
	iface   string        // local IPv4 to bind (empty => 0.0.0.0 / all interfaces)
	bcast   string        // broadcast IPv4 for Who-Is (empty => auto-detect)
	timeout time.Duration // per-request invoke timeout
	retries int           // APDU retries
	delay   time.Duration // pacing delay inserted between sequential requests
	verbose bool          // enable debug logging from the library
}

// app bundles a live client runtime together with the resolved options and a
// cancel function for the background receive loop.
type app struct {
	opts   commonOptions
	rt     *bacnet.ClientRuntime
	cancel context.CancelFunc
}

// newApp constructs and starts a BACnet client runtime from the given options.
// The caller must call app.Close when done.
func newApp(opts commonOptions) (*app, error) {
	if opts.verbose {
		baclog.InitLogger(slog.LevelDebug, false)
	} else {
		// Keep the tool quiet on a production network: this CLI formats and
		// reports all errors itself, so the library's own logging is redundant
		// noise (it logs expected remote errors such as unknown-property at
		// Error level). Discard library logs entirely unless -v is given.
		baclog.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	bindAddr := netip.AddrFrom4([4]byte{0, 0, 0, 0})
	if opts.iface != "" {
		a, err := netip.ParseAddr(opts.iface)
		if err != nil || !a.Is4() {
			return nil, fmt.Errorf("invalid --iface %q: must be an IPv4 address", opts.iface)
		}
		bindAddr = a
	}

	cfg := bacnet.DefaultClientRuntimeConfig()
	if opts.timeout > 0 {
		cfg.ASE.InvokeTimeout = opts.timeout
	}
	if opts.retries >= 0 {
		cfg.ASE.APDURetries = uint8(opts.retries)
	}
	// MaxConcurrentInvokes must stay > 0; DefaultASEConfig already sets 16.

	rt, err := bacnet.NewClientRuntime(bindAddr, cfg)
	if err != nil {
		return nil, fmt.Errorf("create client runtime: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = rt.Run(ctx)
	}()

	return &app{opts: opts, rt: rt, cancel: cancel}, nil
}

// Close stops the background receive loop and releases the UDP socket.
//
// Closing the transport while the receive loop is blocked on a read causes the
// library to log a benign "use of closed network connection" error. Unless the
// user asked for verbose output, we silence the library logger during shutdown
// so this shutdown artifact does not clutter normal output.
func (a *app) Close() {
	if !a.opts.verbose {
		baclog.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.rt != nil {
		_ = a.rt.Close()
	}
}

// client returns the typed APDU client facade.
func (a *app) client() apdu.Client { return a.rt.Client() }

// pace sleeps for the configured inter-request delay, honoring ctx.
// It is a no-op when the delay is zero.
func (a *app) pace(ctx context.Context) {
	if a.opts.delay <= 0 {
		return
	}
	t := time.NewTimer(a.opts.delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// broadcastAddress resolves the Who-Is destination address.
//
// If --bcast was given it is used verbatim. Otherwise the directed broadcast
// address of the interface bound by --iface is auto-detected. If neither is
// available it falls back to the limited broadcast 255.255.255.255.
func (a *app) broadcastAddress() (netprim.Address, error) {
	if a.opts.bcast != "" {
		ap, err := netip.ParseAddrPort(a.opts.bcast)
		if err != nil {
			// Allow a bare IP without a port.
			ip, ipErr := netip.ParseAddr(a.opts.bcast)
			if ipErr != nil || !ip.Is4() {
				return netprim.Address{}, fmt.Errorf("invalid --bcast %q: %w", a.opts.bcast, err)
			}
			ap = netip.AddrPortFrom(ip, defaultUDPPort)
		}
		return netprim.NewAddressFromAddrPort(ap), nil
	}

	bip, err := detectBroadcast(a.opts.iface)
	if err != nil {
		// Fall back to the limited broadcast address.
		return netprim.NewAddressFromAddrPort(
			netip.AddrPortFrom(netip.AddrFrom4([4]byte{255, 255, 255, 255}), defaultUDPPort),
		), nil
	}
	return netprim.NewAddressFromAddrPort(netip.AddrPortFrom(bip, defaultUDPPort)), nil
}

// detectBroadcast computes the IPv4 directed-broadcast address for the given
// local interface IP. If ifaceIP is empty, the first non-loopback IPv4
// interface with a broadcast-capable address is used.
func detectBroadcast(ifaceIP string) (netip.Addr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return netip.Addr{}, err
	}

	var want netip.Addr
	if ifaceIP != "" {
		want, err = netip.ParseAddr(ifaceIP)
		if err != nil {
			return netip.Addr{}, err
		}
	}

	for _, iff := range ifaces {
		if iff.Flags&net.FlagUp == 0 || iff.Flags&net.FlagBroadcast == 0 {
			continue
		}
		if iff.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iff.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			cur, ok := netip.AddrFromSlice(ip4)
			if !ok {
				continue
			}
			// If a specific interface IP was requested, skip others.
			if want.IsValid() && cur != want {
				continue
			}
			bcast := directedBroadcast(ip4, ipnet.Mask)
			b, ok := netip.AddrFromSlice(bcast)
			if !ok {
				continue
			}
			return b, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("no broadcast-capable IPv4 interface found")
}

// directedBroadcast returns ip | ^mask, the directed broadcast address.
func directedBroadcast(ip net.IP, mask net.IPMask) net.IP {
	ip = ip.To4()
	if len(mask) == net.IPv6len {
		mask = mask[12:]
	}
	out := make(net.IP, net.IPv4len)
	for i := 0; i < net.IPv4len; i++ {
		out[i] = ip[i] | ^mask[i]
	}
	return out
}
