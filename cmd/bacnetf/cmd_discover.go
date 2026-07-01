package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"time"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/types"
)

// cmdDiscover implements: bacnetf discover [flags]
//
// It sends a single Who-Is broadcast and collects I-Am responses for a bounded
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
		fmt.Fprintf(fs.Output(), "Broadcast Who-Is and list responding devices.\n\n")
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

	dst, err := a.broadcastAddress()
	if err != nil {
		return err
	}

	fmt.Printf("Discovering on %s (window %s)...\n", dst.AddrPort, window.String())

	ctx, cancel := context.WithTimeout(context.Background(), *window+2*time.Second)
	defer cancel()

	devices, err := a.client().Discover(ctx, apdu.DiscoverRequest{
		Destination: dst,
		WhoIs:       whoIs,
		Window:      *window,
	})
	if err != nil {
		return fmt.Errorf("discover: %s", describeError(err))
	}

	if len(devices) == 0 {
		fmt.Println("No devices responded.")
		return nil
	}

	// Stable output ordered by device instance.
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].DeviceIdentifier.Instance() < devices[j].DeviceIdentifier.Instance()
	})

	fmt.Printf("\nFound %d device(s):\n", len(devices))
	fmt.Printf("%-10s  %-21s  %-8s  %s\n", "DEVICE", "ADDRESS", "VENDOR", "MAX-APDU / SEGMENTATION")
	for _, d := range devices {
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
