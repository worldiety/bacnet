package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/worldiety/bacnet/client"
)

// cmdDiscover implements: bacnetf discover [flags]
//
// It sends a Who-Is to every broadcast-capable interface (or the single
// --bcast/--iface target if given) and lists the responding devices. This is
// the only command that broadcasts; all others are unicast to a single device.
func cmdDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	opts := cliOptions{}
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

	discOpts := []client.DiscoverOption{client.WithWindow(*window)}
	if *lowLimit >= 0 || *highLimit >= 0 {
		if *lowLimit < 0 || *highLimit < 0 {
			return fmt.Errorf("both --low and --high must be set to use a device range")
		}
		discOpts = append(discOpts, client.WithInstanceRange(uint32(*lowLimit), uint32(*highLimit)))
	}

	c, err := newClient(opts)
	if err != nil {
		return err
	}
	defer c.Close()

	// Report which interfaces are being probed.
	if targets, terr := c.BroadcastTargets(); terr == nil {
		labels := make([]string, 0, len(targets))
		for _, t := range targets {
			labels = append(labels, fmt.Sprintf("%s (%s)", t.Address(), t.Label()))
		}
		fmt.Printf("Discovering on %s for %s...\n", strings.Join(labels, ", "), window.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), *window+5*time.Second)
	defer cancel()

	devices, err := c.Discover(ctx, discOpts...)
	if err != nil {
		return fmt.Errorf("discover: %s", client.Describe(err))
	}
	if len(devices) == 0 {
		fmt.Println("No devices responded.")
		return nil
	}

	fmt.Printf("\nFound %d device(s):\n", len(devices))
	fmt.Printf("%-10s  %-21s  %-8s  %s\n", "DEVICE", "ADDRESS", "VENDOR", "MAX-APDU / SEGMENTATION")
	for _, d := range devices {
		fmt.Printf("%-10d  %-21s  %-8d  %d / %s\n",
			d.ID, d.Address, d.Vendor, d.MaxAPDU, d.Segmentation)
	}
	return nil
}
