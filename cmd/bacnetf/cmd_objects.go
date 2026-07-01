package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/common/types"
	bacencoding "github.com/worldiety/bacnet/encoding"
)

// propObjectList is the object-list property identifier (ASHRAE 135).
const propObjectList types.PropertyIdentifier = 76

// cmdObjects implements: bacnetf objects <device> [flags]
//
// It reads the Device object's object-list to enumerate every object on the
// device. The list is read element-by-element (index 0 gives the count, then
// each index in turn) rather than in a single large read, which is gentle on
// slow MS/TP segments and avoids segmentation aborts.
func cmdObjects(args []string) error {
	fs := flag.NewFlagSet("objects", flag.ContinueOnError)
	opts := commonOptions{}
	registerCommonFlags(fs, &opts)

	device := fs.Int("device", -1, "device instance (defaults to the object-list of device:4194303, the 'this device' proxy, if unset)")
	withNames := fs.Bool("names", true, "also read each object's object-name")
	limit := fs.Int("limit", 0, "stop after this many objects (0 = no limit)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: bacnetf objects <device> [flags]\n\n")
		fmt.Fprintf(fs.Output(), "List all objects on a device by reading its object-list.\n\n")
		fmt.Fprintf(fs.Output(), "Examples:\n")
		fmt.Fprintf(fs.Output(), "  bacnetf objects 10.6.6.123 --device 1234\n")
		fmt.Fprintf(fs.Output(), "  bacnetf objects 10.6.6.123 --device 1234 --names=false\n\n")
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

	dst, err := parseDeviceAddr(pos[0])
	if err != nil {
		return err
	}

	// The object-list lives on the Device object. If the caller did not give a
	// device instance we use 4194303 (the reserved "unconfigured / this device"
	// instance) which many devices accept as a self-reference.
	deviceInstance := uint32(4194303)
	if *device >= 0 {
		deviceInstance = uint32(*device)
	}
	deviceOID, err := types.NewObjectIdentifier(types.ObjectTypeDevice, deviceInstance)
	if err != nil {
		return fmt.Errorf("invalid device instance: %w", err)
	}

	a, err := newApp(opts)
	if err != nil {
		return err
	}
	defer a.Close()

	// Step 1: read the array length (index 0).
	count, err := a.readObjectListCount(dst, deviceOID)
	if err != nil {
		return fmt.Errorf("read object-list length of %s: %s", oidLabel(deviceOID), describeError(err))
	}
	fmt.Printf("%s object-list: %d object(s)\n", oidLabel(deviceOID), count)

	if *limit > 0 && uint32(*limit) < count {
		count = uint32(*limit)
		fmt.Printf("(limited to first %d)\n", count)
	}

	// Step 2: read each element by index.
	for i := uint32(1); i <= count; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), a.requestBudget())
		oid, err := a.readObjectListElement(ctx, dst, deviceOID, i)
		cancel()
		if err != nil {
			fmt.Printf("  [%d] ! %s\n", i, describeError(err))
			a.pace(context.Background())
			continue
		}

		if *withNames {
			nctx, ncancel := context.WithTimeout(context.Background(), a.requestBudget())
			name, nerr := a.readProperty(nctx, dst, oid, types.PropertyIdentifierObjectName, nil)
			ncancel()
			if nerr != nil {
				fmt.Printf("  %-26s\n", oidLabel(oid))
			} else {
				fmt.Printf("  %-26s %s\n", oidLabel(oid), name)
			}
			a.pace(context.Background())
			continue
		}

		fmt.Printf("  %s\n", oidLabel(oid))
		a.pace(context.Background())
	}

	return nil
}

// readObjectListCount reads element 0 of object-list, which is the number of
// objects in the list.
func (a *app) readObjectListCount(dst netprim.Address, deviceOID types.ObjectIdentifier) (uint32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.requestBudget())
	defer cancel()

	zero := uint32(0)
	req, err := apdu.NewReadPropertyRequest(deviceOID, propObjectList, &zero)
	if err != nil {
		return 0, err
	}
	ack, err := a.client().ReadProperty(ctx, dst, req)
	if err != nil {
		return 0, err
	}
	val, _, err := bacencoding.DecodeApplicationValue(ack.PropertyValue, 0)
	if err != nil {
		return 0, fmt.Errorf("decode object-list length: %w", err)
	}
	u, ok := val.(bacencoding.AppUnsignedInteger)
	if !ok {
		return 0, fmt.Errorf("object-list length has unexpected type %T", val)
	}
	return uint32(u), nil
}

// readObjectListElement reads element index (1-based) of object-list and
// returns the object identifier stored there.
func (a *app) readObjectListElement(ctx context.Context, dst netprim.Address, deviceOID types.ObjectIdentifier, index uint32) (types.ObjectIdentifier, error) {
	req, err := apdu.NewReadPropertyRequest(deviceOID, propObjectList, &index)
	if err != nil {
		return 0, err
	}
	ack, err := a.client().ReadProperty(ctx, dst, req)
	if err != nil {
		return 0, err
	}
	val, _, err := bacencoding.DecodeApplicationValue(ack.PropertyValue, 0)
	if err != nil {
		return 0, fmt.Errorf("decode object-list element: %w", err)
	}
	oidVal, ok := val.(bacencoding.AppObjectIdentifier)
	if !ok {
		return 0, fmt.Errorf("object-list element has unexpected type %T", val)
	}
	return types.ObjectIdentifier(oidVal), nil
}
