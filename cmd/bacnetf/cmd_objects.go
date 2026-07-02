package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/worldiety/bacnet/client"
)

// cmdObjects implements: bacnetf objects <device> [flags]
//
// It reads the Device object's object-list to enumerate every object on the
// device (element-by-element, gentle on slow MS/TP lines).
func cmdObjects(args []string) error {
	fs := flag.NewFlagSet("objects", flag.ContinueOnError)
	opts := cliOptions{}
	registerCommonFlags(fs, &opts)

	device := fs.Int("device", -1, "device instance for the object-list (only needed when <device> is an IP; ignored when <device> is a device ID)")
	withNames := fs.Bool("names", true, "also read each object's object-name")
	limit := fs.Int("limit", 0, "stop after this many objects (0 = no limit)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf objects <device> [flags]\n\n")
		fmt.Fprintf(fs.Output(), "List all objects on a device by reading its object-list.\n")
		fmt.Fprintf(fs.Output(), "<device> may be a BACnet device ID (e.g. 5123 or device:5123) or an IP.\n")
		fmt.Fprintf(fs.Output(), "When a device ID is given, --device is unnecessary.\n\n")
		fmt.Fprintf(fs.Output(), "Examples:\n")
		fmt.Fprintf(fs.Output(), "  bacnetf objects 5123\n")
		fmt.Fprintf(fs.Output(), "  bacnetf objects 10.6.6.123 --device 5123\n")
		fmt.Fprintf(fs.Output(), "  bacnetf objects 5123 --names=false\n\n")
		fs.PrintDefaults()
	}
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		fs.Usage()
		return fmt.Errorf("expected <device>")
	}

	c, err := newClient(opts)
	if err != nil {
		return err
	}
	defer c.Close()

	ctx := context.Background()
	target, err := resolveAndReport(ctx, c, pos[0])
	if err != nil {
		return err
	}

	objects, err := c.ReadObjectList(ctx, target, client.ObjectListOptions{
		Device: *device,
		Limit:  *limit,
		OnError: func(index uint32, err error) error {
			fmt.Printf("  [%d] ! %s\n", index, client.Describe(err))
			return nil // keep going
		},
	})
	if err != nil {
		return fmt.Errorf("read object-list: %s", client.Describe(err))
	}

	fmt.Printf("object-list: %d object(s)\n", len(objects))

	names := map[client.Object]string{}
	if *withNames {
		names, _ = c.ReadObjectNames(ctx, target, objects)
	}

	for _, obj := range objects {
		if name, ok := names[obj]; ok {
			fmt.Printf("  %-26s %q\n", obj, name)
		} else {
			fmt.Printf("  %s\n", obj)
		}
	}
	return nil
}
