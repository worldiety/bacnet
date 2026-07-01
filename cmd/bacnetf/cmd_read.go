package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/common/types"
)

// cmdRead implements: bacnetf read <device> <object> <property> [flags]
//
// It reads a single property from a single object and prints the value.
func cmdRead(args []string) error {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	opts := commonOptions{}
	registerCommonFlags(fs, &opts)
	index := fs.Int("index", -1, "array index to read (-1 = whole property)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf read <device> <object> <property> [flags]\n\n")
		fmt.Fprintf(fs.Output(), "Read a single property from an object.\n\n")
		fmt.Fprintf(fs.Output(), "Examples:\n")
		fmt.Fprintf(fs.Output(), "  bacnetf read 10.6.6.123 analog-value:1 present-value\n")
		fmt.Fprintf(fs.Output(), "  bacnetf read 10.6.6.123 device:1234 object-name\n\n")
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

	a, err := newApp(opts)
	if err != nil {
		return err
	}
	defer a.Close()

	ctx, cancel := context.WithTimeout(context.Background(), a.requestBudget())
	defer cancel()

	val, err := a.readProperty(ctx, dst, oid, pid, indexPtr(*index))
	if err != nil {
		return fmt.Errorf("read %s %s: %s", oidLabel(oid), propertyName(pid), describeError(err))
	}

	fmt.Printf("%s  %s = %s\n", oidLabel(oid), propertyName(pid), val)
	return nil
}

// defaultPropsFor returns a reasonable set of properties to display for an
// object, tailored a little by object type. These are read one-by-one so a
// single unsupported property does not abort the whole listing.
func defaultPropsFor(oid types.ObjectIdentifier) []types.PropertyIdentifier {
	common := []types.PropertyIdentifier{
		types.PropertyIdentifierObjectName,
		types.PropertyIdentifierObjectType,
		types.PropertyIdentifierDescription,
		types.PropertyIdentifierPresentValue,
		types.PropertyIdentifierStatusFlags,
		types.PropertyIdentifierUnits,
		propByNameOrValue("out-of-service", 81),
		propByNameOrValue("reliability", 103),
	}

	switch oid.ObjectType() {
	case types.ObjectTypeDevice:
		return []types.PropertyIdentifier{
			types.PropertyIdentifierObjectName,
			types.PropertyIdentifierObjectType,
			types.PropertyIdentifierDescription,
			types.PropertyIdentifierVendorName,
			propByNameOrValue("vendor-identifier", 120),
			propByNameOrValue("model-name", 70),
			propByNameOrValue("firmware-revision", 44),
			types.PropertyIdentifierApplicationSoftwareVersion,
			types.PropertyIdentifierProtocolVersion,
			types.PropertyIdentifierProtocolRevision,
			propByNameOrValue("system-status", 112),
			propByNameOrValue("location", 58),
		}
	default:
		return common
	}
}

// cmdProps implements: bacnetf props <device> <object> [property...] [flags]
//
// With no explicit properties it reads a curated default set for the object
// type. Each property is read individually (gentle on slow MS/TP lines and
// robust against unsupported properties).
func cmdProps(args []string) error {
	fs := flag.NewFlagSet("props", flag.ContinueOnError)
	opts := commonOptions{}
	registerCommonFlags(fs, &opts)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf props <device> <object> [property...] [flags]\n\n")
		fmt.Fprintf(fs.Output(), "Read several properties of one object. With no properties given,\n")
		fmt.Fprintf(fs.Output(), "a default set for the object type is read.\n\n")
		fmt.Fprintf(fs.Output(), "Examples:\n")
		fmt.Fprintf(fs.Output(), "  bacnetf props 10.6.6.123 analog-value:1\n")
		fmt.Fprintf(fs.Output(), "  bacnetf props 10.6.6.123 device:1234 object-name model-name firmware-revision\n\n")
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

	dst, err := parseDeviceAddr(pos[0])
	if err != nil {
		return err
	}
	oid, err := parseObjectID(pos[1])
	if err != nil {
		return err
	}

	var pids []types.PropertyIdentifier
	if len(pos) > 2 {
		for _, s := range pos[2:] {
			pid, err := parsePropertyID(s)
			if err != nil {
				return err
			}
			pids = append(pids, pid)
		}
	} else {
		pids = defaultPropsFor(oid)
	}

	a, err := newApp(opts)
	if err != nil {
		return err
	}
	defer a.Close()

	fmt.Printf("%s\n", oidLabel(oid))

	for _, pid := range pids {
		ctx, cancel := context.WithTimeout(context.Background(), a.requestBudget())
		val, err := a.readProperty(ctx, dst, oid, pid, nil)
		cancel()
		if err != nil {
			if isPerObjectError(err) {
				// Property not present / not readable on this object: note briefly.
				fmt.Printf("  %-28s -\n", propertyName(pid))
			} else {
				fmt.Printf("  %-28s ! %s\n", propertyName(pid), describeError(err))
			}
			a.pace(context.Background())
			continue
		}
		fmt.Printf("  %-28s %s\n", propertyName(pid), val)
		a.pace(context.Background())
	}
	return nil
}

// readProperty reads one property and returns it formatted for display.
func (a *app) readProperty(ctx context.Context, dst netprim.Address, oid types.ObjectIdentifier, pid types.PropertyIdentifier, index *uint32) (string, error) {
	req, err := apdu.NewReadPropertyRequest(oid, pid, index)
	if err != nil {
		return "", err
	}
	ack, err := a.client().ReadProperty(ctx, dst, req)
	if err != nil {
		return "", err
	}
	return decodeAndFormat(ack.PropertyValue, pid), nil
}

// requestBudget returns a context timeout for a single request, derived from
// the invoke timeout and retry count so that the context does not fire before
// the library has exhausted its own retries.
func (a *app) requestBudget() time.Duration {
	invoke := a.opts.timeout
	if invoke <= 0 {
		invoke = 10 * time.Second
	}
	retries := a.opts.retries
	if retries < 0 {
		retries = 3
	}
	// (retries+1) attempts, plus a small grace margin.
	return invoke*time.Duration(retries+1) + 2*time.Second
}

// indexPtr converts a -1/absent index flag to a nil pointer, or a value pointer.
func indexPtr(index int) *uint32 {
	if index < 0 {
		return nil
	}
	u := uint32(index)
	return &u
}

// oidLabel renders an object identifier as "type:instance".
func oidLabel(oid types.ObjectIdentifier) string {
	return fmt.Sprintf("%s:%d", objectTypeName(oid.ObjectType()), oid.Instance())
}

// propByNameOrValue returns the property id for a known name, or the fallback
// numeric value when the name is not in the table.
func propByNameOrValue(name string, fallback uint32) types.PropertyIdentifier {
	if pid, ok := propertyByName[name]; ok {
		return pid
	}
	return types.PropertyIdentifier(fallback)
}
