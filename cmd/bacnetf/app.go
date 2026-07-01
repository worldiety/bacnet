package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/worldiety/bacnet"
	"github.com/worldiety/bacnet/apdu"
	baclog "github.com/worldiety/bacnet/common/log"
	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/common/types"
)

// commonOptions holds the flags shared by every subcommand.
type commonOptions struct {
	iface         string        // local IPv4 to bind (empty => 0.0.0.0 / all interfaces)
	bcast         string        // broadcast IPv4 for Who-Is (empty => auto-detect)
	timeout       time.Duration // per-request invoke timeout
	retries       int           // APDU retries
	delay         time.Duration // pacing delay inserted between sequential requests
	resolveWindow time.Duration // how long to wait for I-Am when resolving a device ID
	verbose       bool          // enable debug logging from the library
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

// broadcastTarget describes one Who-Is destination together with a short label
// (the originating interface name, or "manual" for a user-supplied address)
// used only for progress output.
type broadcastTarget struct {
	Label   string
	Address netprim.Address
}

// broadcastTargets resolves the set of Who-Is destinations.
//
// If --bcast was given it is used verbatim as the single target. Otherwise the
// directed broadcast address of every broadcast-capable IPv4 interface is used
// (restricted to --iface if that was given). This lets discovery work on hosts
// that are attached to more than one BACnet network at once. If nothing can be
// detected it falls back to the limited broadcast 255.255.255.255.
func (a *app) broadcastTargets() ([]broadcastTarget, error) {
	if a.opts.bcast != "" {
		ap, err := netip.ParseAddrPort(a.opts.bcast)
		if err != nil {
			// Allow a bare IP without a port.
			ip, ipErr := netip.ParseAddr(a.opts.bcast)
			if ipErr != nil || !ip.Is4() {
				return nil, fmt.Errorf("invalid --bcast %q: %w", a.opts.bcast, err)
			}
			ap = netip.AddrPortFrom(ip, defaultUDPPort)
		}
		return []broadcastTarget{{
			Label:   "manual",
			Address: netprim.NewAddressFromAddrPort(ap),
		}}, nil
	}

	found, err := detectBroadcasts(a.opts.iface)
	if err != nil || len(found) == 0 {
		// Fall back to the limited broadcast address.
		return []broadcastTarget{{
			Label: "limited-broadcast",
			Address: netprim.NewAddressFromAddrPort(
				netip.AddrPortFrom(netip.AddrFrom4([4]byte{255, 255, 255, 255}), defaultUDPPort),
			),
		}}, nil
	}
	return found, nil
}

// resolvedDevice is the result of resolving a device ID to a transport address.
type resolvedDevice struct {
	Instance uint32
	Address  netprim.Address
	VendorID uint16
}

// resolveWindowOrDefault returns the configured resolve window, or 2s if unset.
func (a *app) resolveWindowOrDefault() time.Duration {
	if a.opts.resolveWindow > 0 {
		return a.opts.resolveWindow
	}
	return 2 * time.Second
}

// resolveDevice resolves a BACnet device instance to its transport address by
// sending a Who-Is scoped to exactly that instance on every broadcast target
// and collecting I-Am responses for the resolve window.
//
// Because the Who-Is is limited to a single instance, only the target device
// replies, which keeps discovery traffic minimal on a busy network. It returns
// an error if no device (or more than one device) with that instance answers.
func (a *app) resolveDevice(ctx context.Context, instance uint32) (resolvedDevice, error) {
	di, err := types.NewDeviceInstance(instance)
	if err != nil {
		return resolvedDevice{}, fmt.Errorf("invalid device id %d: %w", instance, err)
	}
	whoIs, err := apdu.NewWhoIsRequestWithLimits(di, di)
	if err != nil {
		return resolvedDevice{}, err
	}

	targets, err := a.broadcastTargets()
	if err != nil {
		return resolvedDevice{}, err
	}

	var (
		mu    sync.Mutex
		seen  = make(map[string]struct{})
		found []resolvedDevice
	)
	err = a.client().RegisterIAmHandler(func(_ context.Context, ind apdu.IAmIndication) error {
		// Only accept the instance we asked for (defensive; some devices ignore
		// the Who-Is range and answer anyway).
		if ind.DeviceIdentifier.Instance() != instance {
			return nil
		}
		key := fmt.Sprintf("%d|%s", ind.Source.Network, ind.Source.AddrPort)
		mu.Lock()
		if _, dup := seen[key]; !dup {
			seen[key] = struct{}{}
			found = append(found, resolvedDevice{
				Instance: ind.DeviceIdentifier.Instance(),
				Address:  ind.Source,
				VendorID: ind.VendorID,
			})
		}
		mu.Unlock()
		return nil
	})
	if err != nil {
		return resolvedDevice{}, fmt.Errorf("register i-am handler: %w", err)
	}

	window := a.resolveWindowOrDefault()
	sendCtx, cancel := context.WithTimeout(ctx, window+2*time.Second)
	defer cancel()

	sent := 0
	for _, t := range targets {
		if err := a.client().WhoIs(sendCtx, t.Address, whoIs); err != nil {
			continue
		}
		sent++
	}
	if sent == 0 {
		return resolvedDevice{}, fmt.Errorf("could not send Who-Is on any interface")
	}

	select {
	case <-sendCtx.Done():
	case <-time.After(window):
	}

	mu.Lock()
	defer mu.Unlock()

	switch len(found) {
	case 0:
		return resolvedDevice{}, fmt.Errorf(
			"device %d not found (no I-Am within %s); is it online and on a reachable network? You can also pass its IP directly",
			instance, window)
	case 1:
		return found[0], nil
	default:
		addrs := make([]string, 0, len(found))
		for _, f := range found {
			addrs = append(addrs, f.Address.AddrPort.String())
		}
		return resolvedDevice{}, fmt.Errorf(
			"device %d answered from multiple addresses (%s); specify the exact IP instead",
			instance, strings.Join(addrs, ", "))
	}
}

// resolveRef turns a parsed <device> argument into a usable transport address.
//
// For an IP reference it returns the address directly. For a device-ID
// reference it performs a scoped Who-Is resolution and prints a one-line notice
// so the operator can see which physical device the ID mapped to. The second
// return value is the device instance when known (from an ID reference), or 0
// for an IP reference where the instance is unknown.
func (a *app) resolveRef(ctx context.Context, ref deviceRef) (netprim.Address, uint32, error) {
	if !ref.IsID {
		return ref.Address, 0, nil
	}
	rd, err := a.resolveDevice(ctx, ref.Instance)
	if err != nil {
		return netprim.Address{}, 0, err
	}
	fmt.Printf("Resolved device %d -> %s\n", rd.Instance, rd.Address.AddrPort)
	return rd.Address, rd.Instance, nil
}

// detectBroadcasts computes the IPv4 directed-broadcast address for every
// broadcast-capable, non-loopback interface that is up. If ifaceIP is set, only
// the interface holding that address is considered. Duplicate broadcast
// addresses are collapsed.
func detectBroadcasts(ifaceIP string) ([]broadcastTarget, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var want netip.Addr
	if ifaceIP != "" {
		want, err = netip.ParseAddr(ifaceIP)
		if err != nil {
			return nil, err
		}
	}

	var targets []broadcastTarget
	seen := make(map[netip.Addr]struct{})

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
			bcastIP := directedBroadcast(ip4, ipnet.Mask)
			b, ok := netip.AddrFromSlice(bcastIP)
			if !ok {
				continue
			}
			if _, dup := seen[b]; dup {
				continue
			}
			seen[b] = struct{}{}
			targets = append(targets, broadcastTarget{
				Label:   iff.Name,
				Address: netprim.NewAddressFromAddrPort(netip.AddrPortFrom(b, defaultUDPPort)),
			})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no broadcast-capable IPv4 interface found")
	}
	return targets, nil
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
