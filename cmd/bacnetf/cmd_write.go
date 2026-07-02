package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/worldiety/bacnet/client"
	"github.com/worldiety/bacnet/common/types"
)

// cmdWrite implements: bacnetf write <device> <object> <property> <type:value> [flags]
//
// Writing mutates a live device. On a production network this is guarded: the
// exact operation is printed and an interactive y/N confirmation is required
// unless --yes is supplied. By default the property is read back after writing.
func cmdWrite(args []string) error {
	fs := flag.NewFlagSet("write", flag.ContinueOnError)
	opts := cliOptions{}
	registerCommonFlags(fs, &opts)

	priority := fs.Int("priority", 0, "write priority 1..16 for commandable properties (0 = none)")
	index := fs.Int("index", -1, "array index to write (-1 = whole property)")
	yes := fs.Bool("yes", false, "skip the interactive confirmation prompt")
	readback := fs.Bool("readback", true, "read the property back after writing")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf write <device> <object> <property> <type:value> [flags]\n\n")
		fmt.Fprintf(fs.Output(), "Write a single property. Mutates a live device; confirmation is required.\n")
		fmt.Fprintf(fs.Output(), "<device> may be a BACnet device ID (e.g. 5123 or device:5123) or an IP.\n\n")
		fmt.Fprintf(fs.Output(), "Value types:\n")
		fmt.Fprintf(fs.Output(), "  null:                 relinquish a commandable property\n")
		fmt.Fprintf(fs.Output(), "  bool:true|false       boolean\n")
		fmt.Fprintf(fs.Output(), "  unsigned:<n>          unsigned integer\n")
		fmt.Fprintf(fs.Output(), "  int:<n>               signed integer\n")
		fmt.Fprintf(fs.Output(), "  real:<f>              single-precision float\n")
		fmt.Fprintf(fs.Output(), "  double:<f>            double-precision float\n")
		fmt.Fprintf(fs.Output(), "  enum:<n>              enumerated\n")
		fmt.Fprintf(fs.Output(), "  string:<text>         character string\n")
		fmt.Fprintf(fs.Output(), "  octet:<hex>           octet string\n")
		fmt.Fprintf(fs.Output(), "  oid:<type>:<inst>     object identifier\n\n")
		fmt.Fprintf(fs.Output(), "Examples:\n")
		fmt.Fprintf(fs.Output(), "  bacnetf write 5123 analog-value:1 present-value real:21.5 --priority 8\n")
		fmt.Fprintf(fs.Output(), "  bacnetf write 10.6.6.123 analog-value:1 present-value null: --priority 8\n\n")
		fs.PrintDefaults()
	}
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 4 {
		fs.Usage()
		return fmt.Errorf("expected <device> <object> <property> <type:value>")
	}

	obj, err := client.ParseObject(pos[1])
	if err != nil {
		return err
	}
	pid, err := client.ParseProperty(pos[2])
	if err != nil {
		return err
	}
	value, err := client.ParseValue(pos[3])
	if err != nil {
		return err
	}

	var writeOpts []client.WriteOption
	if *priority != 0 {
		if *priority < 1 || *priority > 16 {
			return fmt.Errorf("--priority must be between 1 and 16")
		}
		writeOpts = append(writeOpts, client.WithPriority(uint8(*priority)))
	}
	if *index >= 0 {
		writeOpts = append(writeOpts, client.WriteAtIndex(uint32(*index)))
	}

	c, err := newClient(opts)
	if err != nil {
		return err
	}
	defer c.Close()

	ctx := context.Background()

	// Resolve first (for a device ID this discovers its IP) so the confirmation
	// plan shows the exact physical device that will be written. The returned
	// target is address-based, so the write does not re-resolve.
	target, err := resolveAndReport(ctx, c, pos[0])
	if err != nil {
		return err
	}

	printWritePlan(pos[0], target.String(), obj, pid, pos[3], *priority, *index)
	if !*yes {
		ok, err := confirm("Proceed with this write? [y/N] ")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Aborted.")
			return nil
		}
	}

	if *readback {
		v, err := c.WriteAndReadBack(ctx, target, obj, pid, value, writeOpts...)
		if err != nil {
			return fmt.Errorf("write failed: %s", client.Describe(err))
		}
		fmt.Println("Write acknowledged.")
		fmt.Printf("Read-back: %s %s = %s\n", obj, client.PropertyName(pid), v.Display(pid))
		return nil
	}

	if err := c.WriteProperty(ctx, target, obj, pid, value, writeOpts...); err != nil {
		return fmt.Errorf("write failed: %s", client.Describe(err))
	}
	fmt.Println("Write acknowledged.")
	return nil
}

// printWritePlan prints a clear summary of the pending write operation.
func printWritePlan(deviceArg, addr string, obj client.Object, pid types.PropertyIdentifier, rawValue string, prio, index int) {
	fmt.Println("About to WRITE to a live device:")
	fmt.Printf("  Device   : %s (%s)\n", deviceArg, addr)
	fmt.Printf("  Object   : %s\n", obj)
	fmt.Printf("  Property : %s\n", client.PropertyName(pid))
	if index >= 0 {
		fmt.Printf("  Index    : %d\n", index)
	}
	fmt.Printf("  Value    : %s\n", rawValue)
	if prio != 0 {
		fmt.Printf("  Priority : %d\n", prio)
	} else {
		fmt.Printf("  Priority : (none)\n")
	}
}
