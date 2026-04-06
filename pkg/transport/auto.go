package transport

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/model"
)

// AutoDetect tries transports in order: USB → SSH → Cloud
// respects the host from SSHOptions (for WiFi connections)
func AutoDetect(opts ...SSHOption) (Transport, error) {
	// build a temp SSH transport to read the configured host
	probe := NewSSHTransport(opts...)
	host := probe.host

	// try USB Web Interface first (only if on default USB IP)
	if host == "10.11.99.1" {
		if usbAvailable(host) {
			return NewUSBTransport(), nil
		}
	}

	// try SSH at the configured host
	ssh := NewSSHTransport(opts...)
	if err := ssh.Connect(); err == nil {
		return ssh, nil
	}

	// try cloud if tokens exist
	cloud := NewCloudTransport()
	if err := cloud.Connect(); err == nil {
		return cloud, nil
	}

	return nil, model.NewCLIError(model.ErrTransportUnavailable, "",
		fmt.Sprintf("no reMarkable found at %s. Check WiFi or connect via USB.", host))
}

// usbAvailable checks if the USB web interface is reachable
func usbAvailable(host string) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/documents/", host))
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
