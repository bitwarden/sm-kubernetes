/*
Source code in this repository is covered by one of two licenses: (i) the
GNU General Public License (GPL) v3.0 (ii) the Bitwarden License v1.0. The
default license throughout the repository is GPL v3.0 unless the header
specifies another license. Bitwarden Licensed code is found only in the
/bitwarden_license directory.

GPL v3.0:
https://github.com/bitwarden/server/blob/main/LICENSE_GPL.txt

Bitwarden License v1.0:
https://github.com/bitwarden/server/blob/main/LICENSE_BITWARDEN.txt

No grant of any rights in the trademarks, service marks, or logos of Bitwarden is
made (except as may be necessary to comply with the notice requirements as
applicable), and use of any Bitwarden trademarks must comply with Bitwarden
Trademark Guidelines
<https://github.com/bitwarden/server/blob/main/TRADEMARK_GUIDELINES.md>.
*/

// Package csi implements the gRPC server side of the
// kubernetes-sigs/secrets-store-csi-driver provider contract
// (provider/v1alpha1.CSIDriverProviderServer). It is responsible for the
// Version and Mount RPCs the driver calls over a Unix Domain Socket; actual
// retrieval of secrets from Bitwarden Secrets Manager is implemented by a
// later component.
package csi

import (
	"context"

	"github.com/go-logr/logr"
	pb "sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"
)

// ProviderName is reported as the runtime_name in the Version RPC response
// and is used by the driver to identify this provider.
const ProviderName = "bitwarden"

// ProtocolVersion is the provider/v1alpha1 protocol version this server
// implements, echoed back in the Version RPC response.
const ProtocolVersion = "v1alpha1"

// Version is the provider build version reported as runtime_version in the
// Version RPC response. It is intended to be overridden at build time via
// -ldflags, e.g.:
//
//	-ldflags "-X github.com/bitwarden/sm-kubernetes/internal/csi.Version=1.2.3"
var Version = "dev"

// Server implements pb.CSIDriverProviderServer, the gRPC service the
// Secrets Store CSI Driver calls into over a Unix Domain Socket.
type Server struct {
	pb.UnimplementedCSIDriverProviderServer

	Log logr.Logger
}

// NewServer constructs a Server that logs via log.
func NewServer(log logr.Logger) *Server {
	return &Server{Log: log}
}

// Version implements the CSIDriverProvider.Version RPC. It reports this
// provider's name and version, regardless of the version the driver
// requests.
func (s *Server) Version(_ context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	s.Log.V(1).Info("Version called", "driverVersion", req.GetVersion())

	return &pb.VersionResponse{
		Version:        ProtocolVersion,
		RuntimeName:    ProviderName,
		RuntimeVersion: Version,
	}, nil
}

// Mount implements the CSIDriverProvider.Mount RPC. This is currently a
// stub/echo implementation: it acknowledges the request and returns an empty
// result without contacting Bitwarden Secrets Manager. Real secret-fetching
// logic will replace this in a later component.
func (s *Server) Mount(_ context.Context, req *pb.MountRequest) (*pb.MountResponse, error) {
	s.Log.Info("Mount called", "targetPath", req.GetTargetPath())

	return &pb.MountResponse{
		ObjectVersion: []*pb.ObjectVersion{},
		Files:         []*pb.File{},
	}, nil
}
