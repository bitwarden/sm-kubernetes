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
// Version and Mount RPCs the driver calls over a Unix Domain Socket,
// including fetching secrets from Bitwarden Secrets Manager and mapping
// them onto the files returned to the driver.
package csi

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"

	"github.com/bitwarden/sm-kubernetes/internal/sm"
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

	cache *smClientCache
}

// NewServer constructs a Server that logs via log. It maintains a bounded,
// idle-evicting cache of logged-in Secrets Manager sessions so that the
// many concurrent/periodic Mount calls this provider receives as a
// DaemonSet don't each pay the cost of a fresh login and full sync.
func NewServer(log logr.Logger) *Server {
	return newServerWithFactory(log, func(apiURL, identityURL string) sm.BitwardenClientFactory {
		return sm.NewBitwardenClientFactory(apiURL, identityURL)
	})
}

// newServerWithFactory constructs a Server using newFactory to build the
// per-cache-entry sm.BitwardenClientFactory. It is factored out of NewServer
// so tests can substitute a fake factory in place of the real Bitwarden SDK.
func newServerWithFactory(log logr.Logger, newFactory clientFactoryFunc) *Server {
	// defaultStateBaseDir is the fallback parent directory under which the
	// client cache creates an isolated state subdirectory per
	// (organizationId, token) session, used if a fresh temp dir can't be
	// created for some reason.
	defaultStateBaseDir := filepath.Join(os.TempDir(), "bitwarden-csi-provider", "sm-state")

	stateBaseDir := defaultStateBaseDir
	if dir, err := os.MkdirTemp("", "bitwarden-csi-provider-sm-state-"); err == nil {
		stateBaseDir = dir
	}

	return &Server{
		Log:   log,
		cache: newSMClientCache(stateBaseDir, defaultCacheMaxEntries, defaultCacheIdleTimeout, newFactory),
	}
}

// Close releases resources held by the server, including every cached
// Secrets Manager session (closing its SDK client and removing its isolated
// state directory). It is intended to be called once, on provider shutdown.
func (s *Server) Close() {
	if s.cache != nil {
		s.cache.Close()
	}
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

// Mount implements the CSIDriverProvider.Mount RPC. It parses the
// SecretProviderClass parameters and the nodePublishSecretRef-resolved
// Secrets Manager access token, fetches secrets from Bitwarden Secrets
// Manager (via a bounded, single-flighted, per-(organizationId, token)
// cache of logged-in SDK sessions), applies the configured mapping/aliasing
// to produce a file list, and returns it along with stable object versions.
//
// Authentication is always via the per-pod nodePublishSecretRef token; this
// is the only supported (and default) auth path. A shared/provider-level
// token is an explicitly lower-security fallback that is not implemented
// here.
func (s *Server) Mount(ctx context.Context, req *pb.MountRequest) (*pb.MountResponse, error) {
	params, err := ParseParameters(req.GetAttributes())
	if err != nil {
		s.Log.Error(err, "mount failed: invalid SecretProviderClass parameters")
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// token is deliberately never logged; only its cache-key digest
	// (computed inside the cache) is ever retained past this call.
	token, err := extractAccessToken(req.GetSecrets())
	if err != nil {
		s.Log.Error(err, "mount failed: invalid nodePublishSecretRef secret", "organizationId", params.OrganizationID)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	entry, err := s.cache.getOrCreate(params.OrganizationID, token, params.APIURL, params.IdentityURL)
	if err != nil {
		s.Log.Error(err, "mount failed: could not initialize secrets manager session", "organizationId", params.OrganizationID)
		return nil, status.Error(codes.Internal, "failed to initialize a Bitwarden Secrets Manager session")
	}
	// entry is pinned by getOrCreate against concurrent cache eviction; release
	// it once this call is done using it, regardless of outcome, so eviction
	// of this session (once it's no longer in use) can proceed.
	defer entry.release()

	byID, byName, err := entry.sync(ctx)
	if err != nil {
		s.Log.Error(err, "mount failed: secrets manager sync error", "organizationId", params.OrganizationID, "requestedObjectCount", len(params.Objects))
		return nil, status.Error(codes.Unavailable, "failed to sync secrets from Bitwarden Secrets Manager")
	}

	files, versions, err := buildMountFiles(params.Objects, byID, byName)
	if err != nil {
		s.Log.Error(err, "mount failed: could not resolve requested secret objects", "organizationId", params.OrganizationID, "requestedObjectCount", len(params.Objects))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	s.Log.Info("mount succeeded", "organizationId", params.OrganizationID, "objectCount", len(files))

	return &pb.MountResponse{
		ObjectVersion: versions,
		Files:         files,
	}, nil
}
