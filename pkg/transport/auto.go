package transport

import (
	"fmt"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// AutoDetect tries SSH first, falls back to cloud
func AutoDetect(opts ...SSHOption) (Transport, error) {
	// SSH is the primary transport
	ssh := NewSSHTransport(opts...)
	if err := ssh.Connect(); err == nil {
		return ssh, nil
	}

	// cloud fallback
	cloud := NewCloudTransport()
	if err := cloud.Connect(); err == nil {
		return cloud, nil
	}

	probe := NewSSHTransport(opts...)
	return nil, model.NewCLIError(model.ErrTransportUnavailable, "",
		fmt.Sprintf("cannot reach reMarkable at %s via SSH or Cloud", probe.host))
}
