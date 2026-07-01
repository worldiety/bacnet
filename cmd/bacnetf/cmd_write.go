package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/types"
	bacencoding "github.com/worldiety/bacnet/encoding"
)

// cmdWrite implements: bacnetf write <device> <object> <property> <type:value> [flags]
//
// Writing mutates a live device. On a production GA network this is guarded:
// the exact operation is printed and an interactive y/N confirmation is
// required unless --yes is supplied. By default the property is read back after
// writing so the operator can verify the effect.
func cmdWrite(args []string) error {
	fs := flag.NewFlagSet("write", flag.ContinueOnError)
	opts := commonOptions{}
	registerCommonFlags(fs, &opts)

	priority := fs.Int("priority", 0, "write priority 1..16 for commandable properties (0 = none)")
	index := fs.Int("index", -1, "array index to write (-1 = whole property)")
	yes := fs.Bool("yes", false, "skip the interactive confirmation prompt")
	readback := fs.Bool("readback", true, "read the property back after writing")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf write <device> <object> <property> <type:value> [flags]\n\n")
		fmt.Fprintf(fs.Output(), "Write a single property. Mutates a live device; confirmation is required.\n\n")
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
		fmt.Fprintf(fs.Output(), "  bacnetf write 10.6.6.123 analog-value:1 present-value real:21.5 --priority 8\n")
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

	dst, err := parseDeviceAddr(pos[0])
	if err != nil {
		return err
	}
	oid, err := parseObjectID(pos[1])
	if err != nil {
		return err
	}
	pid, err := parsePropertyID(pos[2])
	if err != nil {
		return err
	}
	appVal, err := parseWriteValue(pos[3])
	if err != nil {
		return err
	}

	var prioPtr *uint8
	if *priority != 0 {
		if *priority < 1 || *priority > 16 {
			return fmt.Errorf("--priority must be between 1 and 16")
		}
		p := uint8(*priority)
		prioPtr = &p
	}

	encoded, err := bacencoding.EncodeApplicationValue(appVal)
	if err != nil {
		return fmt.Errorf("encode value: %w", err)
	}

	// Show exactly what will happen and require confirmation.
	printWritePlan(pos[0], dst.AddrPort.String(), oid, pid, pos[3], prioPtr, indexPtr(*index))
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

	a, err := newApp(opts)
	if err != nil {
		return err
	}
	defer a.Close()

	req, err := apdu.NewWritePropertyRequest(oid, pid, indexPtr(*index), encoded, prioPtr)
	if err != nil {
		return fmt.Errorf("build write request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.requestBudget())
	defer cancel()

	if err := a.client().WriteProperty(ctx, dst, req); err != nil {
		return fmt.Errorf("write failed: %s", describeError(err))
	}
	fmt.Println("Write acknowledged.")

	if *readback {
		rctx, rcancel := context.WithTimeout(context.Background(), a.requestBudget())
		defer rcancel()
		val, err := a.readProperty(rctx, dst, oid, pid, indexPtr(*index))
		if err != nil {
			fmt.Printf("Read-back failed: %s\n", describeError(err))
			return nil
		}
		fmt.Printf("Read-back: %s %s = %s\n", oidLabel(oid), propertyName(pid), val)
	}

	return nil
}

// printWritePlan prints a clear summary of the pending write operation.
func printWritePlan(deviceArg, addr string, oid types.ObjectIdentifier, pid types.PropertyIdentifier, rawValue string, prio *uint8, index *uint32) {
	fmt.Println("About to WRITE to a live device:")
	fmt.Printf("  Device   : %s (%s)\n", deviceArg, addr)
	fmt.Printf("  Object   : %s\n", oidLabel(oid))
	fmt.Printf("  Property : %s\n", propertyName(pid))
	if index != nil {
		fmt.Printf("  Index    : %d\n", *index)
	}
	fmt.Printf("  Value    : %s\n", rawValue)
	if prio != nil {
		fmt.Printf("  Priority : %d\n", *prio)
	} else {
		fmt.Printf("  Priority : (none)\n")
	}
}

// confirm prompts the operator and returns true only for an explicit yes.
func confirm(prompt string) (bool, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		// Treat EOF / no tty as "no".
		return false, nil
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}
