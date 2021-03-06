package gohome

import (
	"net"
	"time"

	"github.com/go-home-iot/connection-pool"
	"github.com/go-home-iot/event-bus"
	"github.com/markdaws/gohome/pkg/attr"
	"github.com/markdaws/gohome/pkg/cmd"
)

// Network is an interface that must be exported by an extension that provides
// network related functionality pertaining to the extensions hardware
type Network interface {
	NewConnection(sys *System, d *Device) (func(pool.Config) (net.Conn, error), error)
}

// ExtEvent is a struct that contains a producer and consumer,
// if the extension exports those types
type ExtEvents struct {
	Consumer evtbus.Consumer
	Producer evtbus.Producer
}

type UIField struct {
	// ID a unique ID for the field, doesn't need to be globally unique
	ID string

	// Label is a string that is shown to the user. This should be short, for more
	// information use the Description field
	Label string

	// Description is a string that will be shown to the user.
	Description string

	// Default is the value that shoud be shown by default
	Default string

	// Required indicates the field must be filled out by the user before a scan can occur
	Required bool
}

// DiscovererInfo represents information about a Discoverer instance. Extensions might
// export multiple Discoverers that know how to find devices on a network or create
// devices from config strings
type DiscovererInfo struct {
	// ID a unique ID for the discoverer, extension should put a prefix like "extensionname.hardware.version"
	// etc. as the ID, make it as unique as possible so it won't clash with other Discoverer IDs in other extensions
	ID string

	// Name is a friendly name for the Discoverer, it will be shown in the UI
	Name string

	// Description should contain more info about the discoverer, that wasn't shown in the UI e.g. maybe it
	// explains the discoverer only supports v1.0 of some hardware.
	Description string

	// UIFields is a list of fields that will be displayed to the user before the scan
	// begins, these can be used to get extra information from the user if necessary. For example
	// some system may export a config file but not include the IP address of the device in the
	// export so you can add a UI field to get the user to enter the IP address
	UIFields []UIField

	// PreScanInfo is a string that will be shown to the user before
	// scanning for devices. An example might be some text saying:
	// "Press the sync button on the hub before scanning" - it can be
	// instructions the user should perform before scanning.
	PreScanInfo string
}

// DiscoveryResults contains all of the devices found by the discoverer instance
type DiscoveryResults struct {
	Devices []*Device
	Scenes  []*Scene
}

// Discoverer represents an interface for types that can discover devices on the network or from
// a config file string.
type Discoverer interface {
	Info() DiscovererInfo
	ScanDevices(*System, map[string]string) (*DiscoveryResults, error)
}

// Discovery is the interface exposed by an extension if it supports discovering devices
// on a netowkr or can create devices from a config file
type Discovery interface {
	Discoverers() []DiscovererInfo
	DiscovererFromID(ID string) Discoverer
}

// Extension represents the interface any extension has to implement in order to
// be added to the system
type Extension interface {

	// Name returns a friendly name for the extension
	Name() string

	// BuilderForDevice should return a cmd.Builder if the extension exports a builder
	// for the device that was passed in to the function, nil otherwise
	BuilderForDevice(*System, *Device) cmd.Builder

	// NetworkForDevice should return a gohome.Network if the extension exports a Network interface
	// for the device that was passed in to the function, nil otherwise
	NetworkForDevice(*System, *Device) Network

	// EventsForDevice should return a gohome.ExtEvents instance if the extension supports
	// producing and consuming events for the device passed in to the function
	EventsForDevice(sys *System, d *Device) *ExtEvents

	// Discovery returns a gohome.Discovery instance if the extension can scan for devices
	// on the local network or can create devices from a config file, nil otherwise
	Discovery(sys *System) Discovery
}

// Extensions contains references to all of the loaded extensions in a system
type Extensions struct {
	extensions []Extension
}

// Register adds a new extension to the Extensions instance
func (e *Extensions) Register(ext Extension) {
	e.extensions = append(e.extensions, ext)
}

// FindCmdBuilder returns a cmd.Builder instance if there is any extension that
// exports one for the device passed in to the function
func (e *Extensions) FindCmdBuilder(sys *System, d *Device) cmd.Builder {
	for _, ext := range e.extensions {
		builder := ext.BuilderForDevice(sys, d)
		if builder != nil {
			return builder
		}
	}
	return nil
}

// FindNetwork returns a gohome.Network instance if there is any extension that
// exports one for the device passed in to the function
func (e *Extensions) FindNetwork(sys *System, d *Device) Network {
	for _, ext := range e.extensions {
		network := ext.NetworkForDevice(sys, d)
		if network != nil {
			return network
		}
	}
	return nil
}

// FindEvents returns a gohome.ExtEvents instance if there is any extension
// that supports producing events for the device
func (e *Extensions) FindEvents(sys *System, d *Device) *ExtEvents {
	for _, ext := range e.extensions {
		events := ext.EventsForDevice(sys, d)
		if events != nil {
			return events
		}
	}
	return nil
}

// FindDiscovererFromID returns a Discoverer instance matching the specified ID
func (e *Extensions) FindDiscovererFromID(sys *System, ID string) Discoverer {
	for _, ext := range e.extensions {
		discovery := ext.Discovery(sys)
		if discovery == nil {
			continue
		}

		discoverer := discovery.DiscovererFromID(ID)
		if discoverer != nil {
			return discoverer
		}
	}
	return nil
}

// ListDiscoverers returns a slice containing all of the Discoverers registered with
// all of the extensions in the system
func (e *Extensions) ListDiscoverers(sys *System) []DiscovererInfo {
	allInfos := []DiscovererInfo{}

	for _, ext := range e.extensions {
		disc := ext.Discovery(sys)
		if disc == nil {
			continue
		}
		allInfos = append(allInfos, disc.Discoverers()...)
	}
	return allInfos
}

// NewExtensions inits and returns a new Extensions instance
func NewExtensions() *Extensions {
	exts := &Extensions{}
	return exts
}

// SupressFeatureReporting will stop all FeatureReportingEvt events from being enqueued on the
// system event bus, if they are for the feature that matches the specified featureID. The filter
// will be automatically removed after the specified delay.  The reason you may want to call this
// (see extensions/honeywell/cmd_builder.go) is that for some hardware after you set a new state
// if you query it for the current state you will get the old state not the new state for some period
// of time.  If the extension is polling the hardware for current values and we set new values but then
// report old values, the UI will jump back to an incorrect state. So after setting new values, you
// can call this function with a delay and all updates from the extension will be ignored for that
// time period.
func SupressFeatureReporting(sys *System, featureID string, attrs map[string]*attr.Attribute, delay time.Duration) {

	// If there is an existing filter, remove so we can post the updated values
	sys.Services.EvtBus.RemoveEnqueueFilter(featureID)

	// First, send a reporting event so we have the latest values that were set on the hardware
	if attrs != nil {
		sys.Services.EvtBus.Enqueue(&FeatureReportingEvt{
			FeatureID: featureID,
			Attrs:     attrs,
		})
	}

	// Stop the event bus from accepting any more events for this feature, until the delay has expired, in effect we
	// are ignoring the hardware for a period of time
	sys.Services.EvtBus.AddEnqueueFilter(
		featureID,
		func(e evtbus.Event) bool {
			evt, ok := e.(*FeatureReportingEvt)
			if !ok {
				return false
			}

			if evt.FeatureID == featureID {
				return true
			}
			return false
		},
		delay,
	)
}
