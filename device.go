package gohome

import (
	"fmt"

	"github.com/go-home-iot/connection-pool"
	"github.com/markdaws/gohome/cmd"
	"github.com/markdaws/gohome/validation"
	"github.com/markdaws/gohome/zone"
)

// DeviceType explains the type of a device e.g. Dimmer or Shade
type DeviceType string

const (
	// DTDimmer - dimmer
	DTDimmer DeviceType = "dimmer"

	// DTSwitch - switch
	DTSwitch = "switch"

	// DTShade - shade
	DTShade = "shade"

	// DTHub - hub
	DTHub = "hub"

	// DTRemote - remote control
	DTRemote = "remote"

	// DTUnknown - unknown device type
	DTUnknown = "unknown"
)

// Auth contains authentication information such as login/password/security token
type Auth struct {
	Login    string
	Password string
	Token    string
}

// Device is a piece of hardware. It could be a dimmer, a shade, a remote etc
type Device struct {
	// ID a unique system wide ID, this is set when the device is added to the system
	// don't set this manually unless you know what you are doing
	ID string

	// Address can be whatever type of address is needed for the device e.g.
	// an IP address 192.168.0.9:23 or some value that is custom to whatever
	// system we have imported
	Address string

	// Name is a friendly name for the device, it will be shown in the UI
	Name string

	// Description provides more detailed information about the device
	Description string

	// Type describes what type of device this is e.g. Hub, Switch, Shade etc
	Type DeviceType

	// ModelName
	ModelName string

	// Modelnumber
	ModelNumber string

	// SoftwareVersion can store what version of software is installed on the device
	SoftwareVersion string

	// Buttons - keyed by button address not global ID
	Buttons map[string]*Button

	// Devices - keyed by device address not global ID
	Devices map[string]*Device

	// Zones - keyed by zone address not global ID
	Zones map[string]*zone.Zone

	// Sensor - keyed by sensor address not global ID
	Sensors map[string]*Sensor

	// CmdBuilder knows how to take an abstract command like ZoneSetLevel and turn
	// it in to specific commands for this particular piece of hardware.
	CmdBuilder cmd.Builder

	// Connections is optional, if the device needs a connection pool to communicate.
	Connections *pool.ConnectionPool

	// Auth - if authentication information is required to access the device, it is stored here
	Auth *Auth

	// Hub is the device that should be communicated with to talk to this device.  For example
	// you may have a keypad device, you don't talk directly to that but to some hub which
	// has a network address that then knows how to talk to the keypad.  Calling Hub will give
	// you that device.
	Hub *Device
}

// NewDevice returns an initialized device object
func NewDevice(
	ID,
	name,
	description,
	modelNumber,
	modelName,
	softwareVersion,
	address string,
	hub *Device,
	cmdBuilder cmd.Builder,
	connPool *pool.ConnectionPool,
	auth *Auth) *Device {

	dev := &Device{
		Address:         address,
		ModelNumber:     modelNumber,
		ModelName:       modelName,
		SoftwareVersion: softwareVersion,
		ID:              ID,
		Name:            name,
		Description:     description,
		Hub:             hub,
		Buttons:         make(map[string]*Button),
		Devices:         make(map[string]*Device),
		Zones:           make(map[string]*zone.Zone),
		Sensors:         make(map[string]*Sensor),
		Auth:            auth,
		CmdBuilder:      cmdBuilder,
		Connections:     connPool,
	}
	return dev
}

// Validate checks that all of the requirements for this to be a valid device are met
func (d *Device) Validate() *validation.Errors {
	errors := &validation.Errors{}

	if d.ID == "" {
		errors.Add("required field", "ID")
	}

	if d.Name == "" {
		errors.Add("required field", "Name")
	}

	if d.Address == "" {
		errors.Add("required field", "Address")
	}

	if errors.Has() {
		return errors
	}
	return nil
}

// String returns a friendly string describing the device that can be useful for debugging
func (d *Device) String() string {
	return fmt.Sprintf("Device[ID:%s, Address:%s, Name: %s]", d.ID, d.Address, d.Name)
}

// AddZone adds the zone to the device
func (d *Device) AddZone(z *zone.Zone) error {
	errs := &validation.Errors{}

	// Make sure zone doesn't have same address as any other zone
	if _, ok := d.Zones[z.Address]; ok {
		errs.Add(fmt.Sprintf("device already has a zone with the same address [%s], must be unique", z.Address), "Address")
		return errs
	}

	d.Zones[z.Address] = z
	return nil
}

// AddButton adds a button as a child of this device
func (d *Device) AddButton(b *Button) error {
	if _, ok := d.Buttons[b.Address]; ok {
		return fmt.Errorf("button with address: %s already added to parent device", b.Address)
	}
	d.Buttons[b.Address] = b
	return nil
}

// AddDevice adds a device as a child of this device
func (d *Device) AddDevice(cd *Device) error {
	if _, ok := d.Devices[cd.Address]; ok {
		return fmt.Errorf("device with address: %s already added to parent device", cd.Address)
	}
	d.Devices[cd.Address] = cd
	return nil
}

// AddSensor adds a sensor as a child of this device
func (d *Device) AddSensor(s *Sensor) error {
	if _, ok := d.Sensors[s.Address]; ok {
		return fmt.Errorf("sensor with address: %s already added to device", s.Address)
	}
	d.Sensors[s.Address] = s
	return nil
}

// OwnedZones returns a slice of zones that the device controls, where the
// map is keyed by zone.ID
func (d *Device) OwnedZones(zoneIDs map[string]bool) []*zone.Zone {
	if len(d.Zones) == 0 {
		return nil
	}

	zones := []*zone.Zone{}
	for _, zone := range d.Zones {
		if _, ok := zoneIDs[zone.ID]; ok {
			zones = append(zones, zone)
		}
	}
	return zones
}

// OwnedSensors returns a slice of sensors that the device controls, where the
// map is keyed by sensor.ID
func (d *Device) OwnedSensors(sensorIDs map[string]bool) []*Sensor {
	if len(d.Sensors) == 0 {
		return nil
	}

	sensors := []*Sensor{}
	for _, sensor := range d.Sensors {
		if _, ok := sensorIDs[sensor.ID]; ok {
			sensors = append(sensors, sensor)
		}
	}
	return sensors
}

// IsDupeZone returns true if this zone is a dupe of one the device already owns. This check
// is not based on ID equality, since users could try to import a zone and it is given a new
// ID when it is scanned, but it might be a dupe of a zone we previously scanned, so we have
// to check equality on other properties
func (d *Device) IsDupeZone(z *zone.Zone) (*zone.Zone, bool) {
	zone, ok := d.Zones[z.Address]
	return zone, ok
}

// IsDupeSensor returns true if the sensor is a dupe of one the device already owns
func (d *Device) IsDupeSensor(s *Sensor) (*Sensor, bool) {
	sensor, ok := d.Sensors[s.Address]
	return sensor, ok
}
