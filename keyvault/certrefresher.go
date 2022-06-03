package keyvault

import (
	"context"
	//nolint:gosec // sha1 only used to display cert thumbprint in logs for cross-verification with keyvault.
	"crypto/sha1"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/avast/retry-go/v3"
	"github.com/pkg/errors"
)

type EventualExpirationErr struct {
	time.Time
}

func (e *EventualExpirationErr) Error() string {
	return fmt.Sprintf("could not refresh before expiration on %s", e.Time.String())
}

type tlsCertFetcher interface {
	GetLatestTLSCertificate(ctx context.Context, certName string) (tls.Certificate, error)
}

type logger interface {
	Printf(format string, args ...any)
	Errorf(format string, args ...any)
}

// CertRefresher offers a mechanism to present the latest version of a tls.Certificate from KeyVault, refreshed at an interval.
type CertRefresher struct {
	certName string
	kvc      tlsCertFetcher
	logger   logger

	m    sync.RWMutex
	cert *tls.Certificate
}

// NewCertRefresher returns a CertRefresher. When there's no error, the CertRefresher's GetCertificate method is ready
// for use, returning a valid tls.Certificate fetched from KeyVault during construction.
func NewCertRefresher(ctx context.Context, kvc tlsCertFetcher, l logger, certName string) (*CertRefresher, error) {
	cf := CertRefresher{
		certName: certName,
		kvc:      kvc,
		logger:   l,
	}

	cert, err := cf.kvc.GetLatestTLSCertificate(ctx, cf.certName)
	if err != nil {
		return nil, errors.Wrap(err, "could not fetch initial cert")
	}

	cf.cert = &cert
	cf.logger.Printf("initial certificate fetched: %s", &cf)
	return &cf, nil
}

func (c *CertRefresher) String() string {
	return fmt.Sprintf("cert name: %s, sha1 thumbprint: %s, expiration: %s", c.certName, sha1String(c.cert.Leaf.Raw), c.cert.Leaf.NotAfter.String())
}

// GetCertificate returns the latest certificate fetched from KeyVault.
func (c *CertRefresher) GetCertificate() *tls.Certificate {
	c.m.RLock()
	defer c.m.RUnlock()
	return c.cert
}

// Refresh starts refreshing the certificate at the interval provided.
// It blocks until context is done or refreshing fails.
func (c *CertRefresher) Refresh(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "refresh canceled")
		case <-ticker.C:
			if err := c.refresh(ctx); err != nil {
				c.logger.Errorf("could not refresh before certificate expiration on %s: %v", c.cert.Leaf.NotAfter, err)
				return &EventualExpirationErr{c.cert.Leaf.NotAfter}
			}
		}
	}
}

// refresh will attempt to fetch the latest version of a certificate, up until the current one expires.
func (c *CertRefresher) refresh(ctx context.Context) error {
	certExpires := c.cert.Leaf.NotAfter
	ctx, cancel := context.WithDeadline(ctx, certExpires)
	defer cancel()

	var latestCert tls.Certificate
	retryFn := func() (err error) {
		latestCert, err = c.kvc.GetLatestTLSCertificate(ctx, c.certName)
		if err != nil {
			c.logger.Errorf("could not fetch latest tls certificate: %v. retrying...", err)
			return errors.Wrap(err, "could not fetch latest tls certificate")
		}
		return nil
	}

	if err := retry.Do(retryFn, retry.Context(ctx), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay)); err != nil {
		return errors.Wrap(err, "could not refresh cert")
	}

	c.m.Lock()
	defer c.m.Unlock()

	if latestCert.Leaf.Equal(c.cert.Leaf) {
		c.logger.Printf("certificate unchanged. certificate %s", c)
		return nil
	}

	oldThumbprint := sha1String(c.cert.Leaf.Raw)
	c.cert = &latestCert
	c.logger.Printf("certificate refreshed. old sha1 thumbprint: %s, certificate: %s", oldThumbprint, c)

	return nil
}

func sha1String(bs []byte) string {
	//nolint:gosec // sha1 only used to display cert thumbprint in logs for cross-verification with keyvault.
	return fmt.Sprintf("%X", sha1.Sum(bs))
}
