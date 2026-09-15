package exitcode

import "github.com/KevinLeeNJ/ask/internal/domain/failure"

const (
	Success   = 0
	Internal  = 1
	Usage     = 2
	Config    = 3
	Upstream  = 4
	Transport = 5
	Protocol  = 6
	Canceled  = 130
)

func FromError(err error) int {
	switch failure.KindOf(err) {
	case failure.KindUsage:
		return Usage
	case failure.KindConfig:
		return Config
	case failure.KindUpstream:
		return Upstream
	case failure.KindTransport:
		return Transport
	case failure.KindProtocol:
		return Protocol
	case failure.KindCanceled:
		return Canceled
	default:
		return Internal
	}
}
