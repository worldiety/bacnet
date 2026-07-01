// Command bacnetf (bacnetfinder) is a cross-platform BACnet/IP command-line
// client for discovering devices, exploring their objects and properties, and
// reading and writing property values — a scriptable counterpart to graphical
// tools such as YABE, aimed at commissioning workflows.
//
// Subcommands:
//
//	discover   broadcast Who-Is and list responding devices
//	objects    list all objects on a device (reads its object-list)
//	props      read several properties of one object
//	read       read a single property of an object
//	write      write a single property (guarded; mutates a live device)
//
// Run "bacnetf <command> -h" for command-specific flags.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "discover", "disco":
		err = cmdDiscover(args)
	case "objects", "objs", "ls":
		err = cmdObjects(args)
	case "props", "properties":
		err = cmdProps(args)
	case "read", "rp":
		err = cmdRead(args)
	case "write", "wp":
		err = cmdWrite(args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}

	if err != nil {
		// flag.ErrHelp is returned when the user asked for -h on a subcommand.
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// parseArgs parses a flag set while allowing flags and positional arguments to
// be interspersed in any order (unlike the stdlib flag package, which stops at
// the first positional). It collects positionals into positional and feeds the
// flags to fs. This lets commands such as
//
//	bacnetf write 10.6.6.123 analog-value:1 present-value real:21.5 --priority 8
//
// work with the flag placed after the positional operands.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		// The first leftover is a positional (Parse stops at non-flags).
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	return positional, nil
}

// registerCommonFlags registers the flags shared by every subcommand.
func registerCommonFlags(fs *flag.FlagSet, opts *commonOptions) {
	fs.StringVar(&opts.iface, "iface", "", "local IPv4 to bind (default: all interfaces; recommended for discovery so broadcast I-Am replies are received)")
	fs.StringVar(&opts.bcast, "bcast", "", "broadcast IPv4 for Who-Is (default: auto-detect every interface)")
	fs.DurationVar(&opts.timeout, "timeout", 10*time.Second, "per-request timeout (raise for slow MS/TP lines)")
	fs.IntVar(&opts.retries, "retries", 3, "APDU retransmission attempts")
	fs.DurationVar(&opts.delay, "delay", 0, "delay inserted between sequential requests (gentle pacing for MS/TP)")
	fs.BoolVar(&opts.verbose, "v", false, "verbose: enable debug logging from the BACnet stack")
}

func usage() {
	fmt.Fprintf(os.Stderr, `bacnetf (bacnetfinder) - cross-platform BACnet/IP CLI

Usage:
  bacnetf <command> [arguments] [flags]

Commands:
  discover                                   broadcast Who-Is and list devices
  objects  <device>                          list all objects on a device
  props    <device> <object> [property...]   read properties of an object
  read     <device> <object> <property>      read a single property
  write    <device> <object> <property> <type:value>
                                             write a single property (guarded)

Addressing:
  <device>    IPv4[:port]         e.g. 10.6.6.123 or 10.6.6.123:47808
  <object>    <type>:<instance>   e.g. analog-value:1, device:1234, 2:1
  <property>  <name>|<number>     e.g. present-value, object-name, 85

Examples:
  bacnetf discover
  bacnetf objects 10.6.6.123 --device 1234
  bacnetf props   10.6.6.123 device:1234
  bacnetf read    10.6.6.123 analog-value:1 present-value
  bacnetf write   10.6.6.123 analog-value:1 present-value real:21.5 --priority 8

Run "bacnetf <command> -h" for command-specific flags.
`)
}
