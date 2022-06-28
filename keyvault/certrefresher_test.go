package keyvault

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertRefresher(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()

	// returns a different cert on every invocation, until context is done
	tlsFn := tlsFunc(func() (tls.Certificate, error) {
		if err := ctx.Err(); err != nil {
			return tls.Certificate{}, errors.Wrap(err, "context done")
		}

		bs := make([]byte, 100)
		_, _ = rand.Read(bs)
		leaf := x509.Certificate{Raw: bs, NotAfter: time.Now().Add(time.Minute)}

		return tls.Certificate{Leaf: &leaf}, nil
	})

	cf, err := NewCertRefresher(ctx, tlsFn, testLogger{t}, "dummy")
	require.NoError(t, err)

	// a new cert should be loaded roughly every second
	go func() { _ = cf.Refresh(ctx, time.Second) }()

	thumbprintSet := stringSet{ts: make(map[string]struct{})}

	// spin multiple concurrent readers, collecting unique thumbprints for eventual assertion
	for i := 0; i < 10; i++ {
		go readAndCollect(ctx, cf, &thumbprintSet, time.Millisecond*300)
	}

	waitFor := time.Second * 10
	// at least this many unique certs should eventually be seen
	condFn := func() bool { return thumbprintSet.len() > 5 }
	checkEvery := time.Second

	assert.Eventually(t, condFn, waitFor, checkEvery)
}

func TestCertRefresher_RetryUntilExpiration(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()

	called := false
	// returns a cert with short expiration once, then consistently errors
	tlsFn := tlsFunc(func() (tls.Certificate, error) {
		if called {
			return tls.Certificate{}, errors.New("some error")
		}
		called = true
		leaf := x509.Certificate{Raw: []byte{0}, NotAfter: time.Now().Add(time.Second * 5)}
		return tls.Certificate{Leaf: &leaf}, nil
	})

	cf, err := NewCertRefresher(ctx, tlsFn, testLogger{t}, "dummy")
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() { errCh <- cf.Refresh(ctx, time.Second) }()

	waitFor := time.Second * 10
	condFn := func() bool {
		select {
		case err := <-errCh:
			var expErr *EventualExpirationErr
			return errors.As(err, &expErr)
		default:
		}
		return false
	}
	checkEvery := time.Second

	assert.Eventually(t, condFn, waitFor, checkEvery)
}

type tlsFunc func() (tls.Certificate, error)

func (t tlsFunc) GetLatestTLSCertificate(_ context.Context, _ string) (tls.Certificate, error) {
	return t()
}

type stringSet struct {
	sync.RWMutex
	ts map[string]struct{}
}

func (s *stringSet) add(val string) {
	s.Lock()
	s.ts[val] = struct{}{}
	s.Unlock()
}

func (s *stringSet) len() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.ts)
}

func readAndCollect(ctx context.Context, cf *CertRefresher, thumbprintSet *stringSet, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cert := cf.GetCertificate()
			thumbprintSet.add(sha1String(cert.Leaf.Raw))
		}
	}
}

type testLogger struct{ *testing.T }

func (t testLogger) Printf(format string, args ...any) {
	t.Logf(format, args...)
}

func (t testLogger) Errorf(format string, args ...any) {
	t.Logf(format, args...)
}

// todo: move to a better package for reuse
func testContext(t *testing.T) (context.Context, context.CancelFunc) {
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		return context.WithDeadline(ctx, deadline)
	}
	return context.WithCancel(ctx)
}
