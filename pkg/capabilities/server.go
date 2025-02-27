package capabilities

import (
	"context"

	remoteexecution "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/bazelbuild/remote-apis/build/bazel/semver"
	"github.com/buildbarn/bb-storage/pkg/digest"
	"github.com/buildbarn/bb-storage/pkg/util"

	//"google.golang.org/protobuf/proto"
	"github.com/golang/protobuf/proto"
)

type server struct {
	provider Provider
}

type CapabilitiesServer interface {
	// GetCapabilities returns the server capabilities configuration of the
	// remote endpoint.
	// Only the capabilities of the services supported by the endpoint will
	// be returned:
	// * Execution + CAS + Action Cache endpoints should return both
	//   CacheCapabilities and ExecutionCapabilities.
	// * Execution only endpoints should return ExecutionCapabilities.
	// * CAS + Action Cache only endpoints should return CacheCapabilities.
	GetCapabilities(context.Context, *remoteexecution.GetCapabilitiesRequest) (*remoteexecution.ServerCapabilities, error)
}

// NewServer creates a gRPC server object for the REv2 Capabilities service.
//func NewServer(provider Provider) remoteexecution.CapabilitiesServer {
func NewServer(provider Provider) CapabilitiesServer {
	return &server{
		provider: provider,
	}
}

func (s *server) GetCapabilities(ctx context.Context, in *remoteexecution.GetCapabilitiesRequest) (*remoteexecution.ServerCapabilities, error) {
	instanceName, err := digest.NewInstanceName(in.InstanceName)
	if err != nil {
		return nil, util.StatusWrapf(err, "Invalid instance name %#v", in.InstanceName)
	}

	capabilities, err := s.provider.GetCapabilities(ctx, instanceName)
	if err != nil {
		return nil, err
	}

	// TODO: Set these version numbers properly; including
	// DeprecatedApiVersion. Instead of setting these here, should
	// we let providers set these and have MergingProvider merge
	// those as well?
	capabilitiesWithVersion := remoteexecution.ServerCapabilities{
		LowApiVersion:  &semver.SemVer{Major: 2},
		HighApiVersion: &semver.SemVer{Major: 2},
	}
	proto.Merge(&capabilitiesWithVersion, capabilities)
	return &capabilitiesWithVersion, nil
}
