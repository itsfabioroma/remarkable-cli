package transport

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/model"
)

// AutoDetect tries transports in order: USB → SSH → Cloud
func AutoDetect(opts ...SSHOption) (Transport, error) {
	// try USB Web Interface first (no dev mode needed)
	if usbAvailable() {
		return NewUSBTransport(), nil
	}

	// try SSH (dev mode required)
	ssh := NewSSHTransport(opts...)
	if err := ssh.Connect(); err == nil {
		return ssh, nil
	}

	// TODO: try cloud if tokens exist

	return nil, model.NewCLIError(model.ErrTransportUnavailable, "",
		"no reMarkable device found. Connect via USB cable or ensure SSH is available at 10.11.99.1")
}

// usbAvailable checks if the USB web interface is reachable
func usbAvailable() bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get("http://10.11.99.1/documents/")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// sshAvailable checks if SSH port is open
func sshAvailable(host string) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:22", host), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
