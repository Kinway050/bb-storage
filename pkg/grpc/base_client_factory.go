package grpc

import (
	"context"
	"net/http"
	"net/url"

	configuration "github.com/buildbarn/bb-storage/pkg/proto/configuration/grpc"
	"github.com/buildbarn/bb-storage/pkg/util"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

type baseClientFactory struct {
	dialer             ClientDialer
	unaryInterceptors  []grpc.UnaryClientInterceptor
	streamInterceptors []grpc.StreamClientInterceptor
}

// NewBaseClientFactory creates factory for gRPC clients that calls into
// ClientDialer to construct the actual client.
func NewBaseClientFactory(dialer ClientDialer, unaryInterceptors []grpc.UnaryClientInterceptor, streamInterceptors []grpc.StreamClientInterceptor) ClientFactory {
	// Limit slice capacity to length, so that any appending
	// additional interceptors always triggers a copy.
	return baseClientFactory{
		dialer:             dialer,
		unaryInterceptors:  unaryInterceptors[:len(unaryInterceptors):len(unaryInterceptors)],
		streamInterceptors: streamInterceptors[:len(streamInterceptors):len(streamInterceptors)],
	}
}

func (cf baseClientFactory) NewClientFromConfiguration(config *configuration.ClientConfiguration) (grpc.ClientConnInterface, error) {
	if config == nil {
		return nil, status.Error(codes.InvalidArgument, "No gRPC client configuration provided")
	}

	var dialOptions []grpc.DialOption
	unaryInterceptors := cf.unaryInterceptors
	streamInterceptors := cf.streamInterceptors

	// Optional: TLS.
	tlsConfig, err := util.NewTLSConfigFromClientConfiguration(config.Tls)
	if err != nil {
		return nil, util.StatusWrap(err, "Failed to create TLS configuration")
	}
	if tlsConfig != nil {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}

	if windowSize := config.InitialWindowSizeBytes; windowSize != 0 {
		dialOptions = append(dialOptions, grpc.WithInitialWindowSize(windowSize))
	}
	if connWindowSize := config.InitialConnWindowSizeBytes; connWindowSize != 0 {
		dialOptions = append(dialOptions, grpc.WithInitialConnWindowSize(connWindowSize))
	}

	// Optional: OAuth authentication.
	if oauthConfig := config.Oauth; oauthConfig != nil {
		var perRPC credentials.PerRPCCredentials
		var err error
		switch credentials := oauthConfig.Credentials.(type) {
		case *configuration.ClientOAuthConfiguration_GoogleDefaultCredentials:
			perRPC, err = oauth.NewApplicationDefault(context.Background(), oauthConfig.Scopes...)
		case *configuration.ClientOAuthConfiguration_ServiceAccountKey:
			perRPC, err = oauth.NewServiceAccountFromKey([]byte(credentials.ServiceAccountKey), oauthConfig.Scopes...)
		default:
			return nil, status.Error(codes.InvalidArgument, "gRPC client credentials are wrong: one of googleDefaultCredentials or serviceAccountKey should be provided")
		}
		if err != nil {
			return nil, util.StatusWrap(err, "Failed to create gRPC credentials")
		}
		dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(perRPC))
	}

	// Optional: Keepalive.
	if config.Keepalive != nil {
		time := config.Keepalive.Time
		if err := time.CheckValid(); err != nil {
			return nil, util.StatusWrap(err, "Failed to parse keepalive time")
		}
		timeout := config.Keepalive.Timeout
		if err := timeout.CheckValid(); err != nil {
			return nil, util.StatusWrap(err, "Failed to parse keepalive timeout")
		}
		dialOptions = append(dialOptions, grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                time.AsDuration(),
			Timeout:             timeout.AsDuration(),
			PermitWithoutStream: config.Keepalive.PermitWithoutStream,
		}))
	}

	// Optional: metadata forwarding.
	if headers := config.ForwardMetadata; len(headers) > 0 {
		unaryInterceptors = append(
			unaryInterceptors,
			NewMetadataForwardingUnaryClientInterceptor(headers))
		streamInterceptors = append(
			streamInterceptors,
			NewMetadataForwardingStreamClientInterceptor(headers))
	}

	// Optional: add metadata.
	if headers := config.AddMetadata; len(headers) > 0 {
		var headerValues MetadataHeaderValues
		for _, entry := range headers {
			headerValues.Add(entry.Header, entry.Values)
		}
		unaryInterceptors = append(
			unaryInterceptors,
			NewMetadataAddingUnaryClientInterceptor(headerValues))
		streamInterceptors = append(
			streamInterceptors,
			NewMetadataAddingStreamClientInterceptor(headerValues))
	}

	// Optional: proxying.
	if proxyURL := config.ProxyUrl; proxyURL != "" {
		parsedProxyURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, util.StatusWrap(err, "Failed to parse proxy URL")
		}
		proxyDialer := proxyDialer{
			httpProxy: http.ProxyURL(parsedProxyURL),
		}
		dialOptions = append(dialOptions, grpc.WithContextDialer(proxyDialer.proxyDial))
	}

	dialOptions = append(
		dialOptions,
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...))
	return cf.dialer(context.Background(), config.Address, dialOptions...)
}
