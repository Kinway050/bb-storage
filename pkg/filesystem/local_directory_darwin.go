// +build darwin

package filesystem

import (
	"os"

	"github.com/buildbarn/bb-storage/pkg/filesystem/path"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// deviceNumber is the equivalent of POSIX dev_t.
type deviceNumber = int32

func (d *localDirectory) Mknod(name path.Component, perm os.FileMode, dev int) error {
	return status.Error(codes.Unimplemented, "Creation of device nodes is not supported on Darwin")
}
