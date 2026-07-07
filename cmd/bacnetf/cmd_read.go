package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/worldiety/bacnet/client"
	"github.com/worldiety/bacnet/common/types"
)

// resolveAndReport turns a device argument into an addressable target. When the
// argument is a device ID, it resolves the address up-front (via the service)
// and prints a one-line notice, then returns an address-based target so that
// subsequent calls in the same command do not re-resolve.
func resolveAndReport(ctx context.Context, c *client.Client, deviceArg string) (client.Target, error) {
	target, err := client.ParseTarget(deviceArg)
	if err != nil {
		return client.Target{}, err
	}
	if !target.IsID() {
		return target, nil
	}
	d, err := c.Resolve(ctx, target.Instance())
	if err != nil {
		return client.Target{}, err
	}
	fmt.Printf("Resolved device %d -> %s\n", d.ID, d.Address)
	return d.Target(), nil
}

// cmdRead implements: bacnetf read <device> <object> <property> [flags]
func cmdRead(args []string) error {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	opts := cliOptions{}
	registerCommonFlags(fs, &opts)
	index := fs.Int("index", -1, "array index to read (-1 = whole property)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf read <device> <object> <property> [flags]\n\n")
		fmt.Fprintf(fs.Output(), "Read a single property from an object.\n")
		fmt.Fprintf(fs.Output(), "<device> may be a BACnet device ID (e.g. 5123 or device:5123) or an IP.\n\n")
		fmt.Fprintf(fs.Output(), "Examples:\n")
		fmt.Fprintf(fs.Output(), "  bacnetf read 5123 analog-value:1 present-value\n")
		fmt.Fprintf(fs.Output(), "  bacnetf read 10.6.6.123 device:5123 object-name\n\n")
		fs.PrintDefaults()
	}
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 3 {
		fs.Usage()
		return fmt.Errorf("expected <device> <object> <property>")
	}

	obj, err := client.ParseObject(pos[1])
	if err != nil {
		return err
	}
	pid, err := client.ParseProperty(pos[2])
	if err != nil {
		return err
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

	var readOpts []client.ReadOption
	if *index >= 0 {
		readOpts = append(readOpts, client.AtIndex(uint32(*index)))
	}

	v, err := c.ReadProperty(ctx, target, obj, pid, readOpts...)
	if err != nil {
		return fmt.Errorf("read %s %s: %s", obj, client.PropertyName(pid), client.Describe(err))
	}

	fmt.Printf("%s  %s = %s\n", obj, client.PropertyName(pid), v.Display(pid))
	return nil
}

// cmdProps implements: bacnetf props <device> <object> [property...] [flags]
//
// With no explicit properties it reads a curated default set for the object
// type. By default it uses a single ReadPropertyMultiple request and falls back
// automatically to per-property reads for devices that do not support it; pass
// --no-rpm to always read each property individually.
func cmdProps(args []string) error {
	fs := flag.NewFlagSet("props", flag.ContinueOnError)
	opts := cliOptions{}
	registerCommonFlags(fs, &opts)
	noRPM := fs.Bool("no-rpm", false, "read each property individually instead of ReadPropertyMultiple")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf props <device> <object> [property...] [flags]\n\n")
		fmt.Fprintf(fs.Output(), "Read several properties of one object. With no properties given,\n")
		fmt.Fprintf(fs.Output(), "a default set for the object type is read. Uses ReadPropertyMultiple\n")
		fmt.Fprintf(fs.Output(), "by default, falling back to per-property reads when unsupported.\n")
		fmt.Fprintf(fs.Output(), "<device> may be a BACnet device ID (e.g. 5123 or device:5123) or an IP.\n\n")
		fmt.Fprintf(fs.Output(), "Examples:\n")
		fmt.Fprintf(fs.Output(), "  bacnetf props 5123 analog-value:1\n")
		fmt.Fprintf(fs.Output(), "  bacnetf props 5123 device:5123 object-name model-name firmware-revision\n\n")
		fs.PrintDefaults()
	}
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		fs.Usage()
		return fmt.Errorf("expected <device> <object> [property...]")
	}

	obj, err := client.ParseObject(pos[1])
	if err != nil {
		return err
	}

	var pids []types.PropertyIdentifier
	if len(pos) > 2 {
		for _, s := range pos[2:] {
			pid, err := client.ParseProperty(s)
			if err != nil {
				return err
			}
			pids = append(pids, pid)
		}
	} else {
		pids = client.DefaultProperties(obj)
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

	var (
		results []client.PropertyResult
	)
	if *noRPM {
		results, err = c.ReadProperties(ctx, target, obj, pids)
	} else {
		results, err = c.ReadPropertiesMultiple(ctx, target, obj, pids)
	}
	if err != nil {
		return fmt.Errorf("read %s: %s", obj, client.Describe(err))
	}

	fmt.Printf("%s\n", obj)
	for _, r := range results {
		name := client.PropertyName(r.Property)
		if r.Err != nil {
			if client.IsRemoteError(r.Err) {
				fmt.Printf("  %-28s -\n", name)
			} else {
				fmt.Printf("  %-28s ! %s\n", name, client.Describe(r.Err))
			}
			continue
		}
		fmt.Printf("  %-28s %s\n", name, r.Value.Display(r.Property))
	}
	return nil
}
