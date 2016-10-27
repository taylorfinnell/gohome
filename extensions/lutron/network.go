package lutron

import (
	"bufio"
	"errors"
	"fmt"
	"net"

	"github.com/go-home-iot/connection-pool"
	"github.com/markdaws/gohome"
	"github.com/markdaws/gohome/log"
)

type network struct {
	System *gohome.System
}

func (d *network) Devices(sys *gohome.System, modelNumber string) ([]*gohome.Device, error) {
	return nil, errors.New("unsupported method - Devices")
}

func (d *network) NewConnection(sys *gohome.System, dev *gohome.Device) (func(pool.Config) (net.Conn, error), error) {
	return func(cfg pool.Config) (net.Conn, error) {
		log.V("Attempting to connect to Device[%s] %s", dev.Name, dev.Address)

		conn, err := net.Dial("tcp", dev.Address)
		if err != nil {
			log.E("Failed to connect to Device[%s] %s, %s", dev.Name, dev.Address, err)
			return nil, err
		}

		r := bufio.NewReader(conn)
		_, err = r.ReadString(':')
		if err != nil {
			return nil, fmt.Errorf("authenticate login failed: %s", err)
		}

		_, err = conn.Write([]byte(dev.Auth.Login + "\r\n"))
		if err != nil {
			return nil, fmt.Errorf("authenticate write login failed: %s", err)
		}

		_, err = r.ReadString(':')
		if err != nil {
			return nil, fmt.Errorf("authenticate password failed: %s", err)
		}

		_, err = conn.Write([]byte(dev.Auth.Password + "\r\n"))
		if err != nil {
			return nil, fmt.Errorf("authenticate password failed: %s", err)
		}

		log.V("Connected to Device[%s] %s", dev.Name, dev.Address)
		return conn, nil
	}, nil
}