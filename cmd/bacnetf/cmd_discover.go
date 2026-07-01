package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/types"
)

// cmdDiscover implements: bacnetf discover [flags]
//
// It sends a Who-Is to every broadcast-capable interface (or the single
// --bcast/--iface target if given) and collects I-Am responses for a bounded
// window, then prints the devices found. This is the only command that
// broadcasts; all others are unicast to a single device.
func cmdDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	opts := commonOptions{}
	registerCommonFlags(fs, &opts)

	window := fs.Duration("window", 5*time.Second, "how long to collect I-Am responses")
	lowLimit := fs.Int("low", -1, "optional lowest device instance to query (-1 = unbounded)")
	highLimit := fs.Int("high", -1, "optional highest device instance to query (-1 = unbounded)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf discover [flags]\n\n")
		fmt.Fprintf(fs.Output(), "Broadcast Who-Is and list responding devices.\n")
		fmt.Fprintf(fs.Output(), "By default a Who-Is is sent on every broadcast-capable interface.\n\n")
		fs.PrintDefaults()
	}
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		fs.Usage()
		return fmt.Errorf("discover takes no positional arguments (got %v)", pos)
	}

	whoIs, err := buildWhoIs(*lowLimit, *highLimit)
	if err != nil {
		return err
	}

	a, err := newApp(opts)
	if err != nil {
		return err
	}
	defer a.Close()

	targets, err := a.broadcastTargets()
	if err != nil {
		return err
	}

	// Collect I-Am responses via a registered handler so that a single
	// collection window can span Who-Is requests sent to several networks.
	var (
		mu      sync.Mutex
		seen    = make(map[string]struct{})
		devices []apdu.IAmIndication
	)
	err = a.client().RegisterIAmHandler(func(_ context.Context, ind apdu.IAmIndication) error {
		key := fmt.Sprintf("%d|%d|%s", ind.DeviceIdentifier, ind.Source.Network, ind.Source.AddrPort)
		mu.Lock()
		if _, dup := seen[key]; !dup {
			seen[key] = struct{}{}
			devices = append(devices, ind)
		}
		mu.Unlock()
		return nil
	})
	if err != nil {
		return fmt.Errorf("register i-am handler: %w", err)
	}

	labels := make([]string, 0, len(targets))
	for _, t := range targets {
		labels = append(labels, fmt.Sprintf("%s (%s)", t.Address.AddrPort, t.Label))
	}
	fmt.Printf("Discovering on %s for %s...\n", strings.Join(labels, ", "), window.String())

	ctx, cancel := context.WithTimeout(context.Background(), *window+2*time.Second)
	defer cancel()

	// Send Who-Is to each target. Failures on one interface must not abort the
	// others (e.g. a down link or a directed broadcast the OS refuses).
	sent := 0
	for _, t := range targets {
		if err := a.client().WhoIs(ctx, t.Address, whoIs); err != nil {
			fmt.Printf("  warning: Who-Is on %s failed: %s\n", t.Address.AddrPort, describeError(err))
			continue
		}
		sent++
	}
	if sent == 0 {
		return fmt.Errorf("could not send Who-Is on any interface")
	}

	// Collect for the window.
	select {
	case <-ctx.Done():
	case <-time.After(*window):
	}

	mu.Lock()
	collected := make([]apdu.IAmIndication, len(devices))
	copy(collected, devices)
	mu.Unlock()

	if len(collected) == 0 {
		fmt.Println("No devices responded.")
		return nil
	}

	// Stable output ordered by device instance.
	sort.Slice(collected, func(i, j int) bool {
		return collected[i].DeviceIdentifier.Instance() < collected[j].DeviceIdentifier.Instance()
	})

	fmt.Printf("\nFound %d device(s):\n", len(collected))
	fmt.Printf("%-10s  %-21s  %-8s  %s\n", "DEVICE", "ADDRESS", "VENDOR", "MAX-APDU / SEGMENTATION")
	for _, d := range collected {
		fmt.Printf("%-10d  %-21s  %-8d  %v / %v\n",
			d.DeviceIdentifier.Instance(),
			d.Source.AddrPort,
			d.VendorID,
			d.MaxAPDULengthAccepted,
			d.SegmentationSupported,
		)
	}
	return nil
}

// buildWhoIs constructs a Who-Is request, applying an optional instance range.
// A range requires both bounds; -1 means "unbounded".
func buildWhoIs(low, high int) (apdu.WhoIsRequest, error) {
	if low < 0 && high < 0 {
		return apdu.NewWhoIsRequest(), nil
	}
	if low < 0 || high < 0 {
		return apdu.WhoIsRequest{}, fmt.Errorf("both --low and --high must be set to use a device range")
	}
	lo, err := types.NewDeviceInstance(uint32(low))
	if err != nil {
		return apdu.WhoIsRequest{}, fmt.Errorf("invalid --low: %w", err)
	}
	hi, err := types.NewDeviceInstance(uint32(high))
	if err != nil {
		return apdu.WhoIsRequest{}, fmt.Errorf("invalid --high: %w", err)
	}
	return apdu.NewWhoIsRequestWithLimits(lo, hi)
}
