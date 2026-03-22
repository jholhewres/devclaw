package commands

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	devtls "github.com/jholhewres/devclaw/pkg/devclaw/tls"
	"github.com/spf13/cobra"
)

// newTLSCmd creates the `devclaw tls` command group for TLS certificate management.
func newTLSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tls",
		Short: "Manage TLS certificates",
		Long:  `Generate and inspect self-signed TLS certificates for DevClaw HTTPS.`,
	}

	cmd.AddCommand(
		newTLSGenerateCmd(),
		newTLSInfoCmd(),
	)

	return cmd
}

// newTLSGenerateCmd creates the `devclaw tls generate` subcommand.
func newTLSGenerateCmd() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a self-signed TLS certificate",
		Long: `Generate a self-signed ECDSA P-256 TLS certificate for DevClaw.
The certificate is valid for 10 years and includes localhost/127.0.0.1 as SANs.

Examples:
  devclaw tls generate
  devclaw tls generate --output /etc/devclaw/tls`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if outputDir == "" {
				outputDir = filepath.Join("data", "tls")
			}

			certPath := filepath.Join(outputDir, "devclaw-cert.pem")
			keyPath := filepath.Join(outputDir, "devclaw-key.pem")

			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

			// Remove existing files to force regeneration.
			for _, p := range []string{certPath, keyPath} {
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("removing existing %s: %w", p, err)
				}
			}

			if err := devtls.EnsureSelfSignedCert(certPath, keyPath, logger); err != nil {
				return fmt.Errorf("failed to generate certificate: %w", err)
			}

			fp, err := devtls.CertFingerprint(certPath)
			if err != nil {
				return fmt.Errorf("failed to read fingerprint: %w", err)
			}

			expiry, err := devtls.CertExpiry(certPath)
			if err != nil {
				return fmt.Errorf("failed to read expiry: %w", err)
			}

			fmt.Println()
			fmt.Println("  TLS certificate generated successfully!")
			fmt.Println()
			fmt.Printf("  Certificate: %s\n", certPath)
			fmt.Printf("  Private key: %s\n", keyPath)
			fmt.Printf("  Fingerprint: %s\n", fp)
			fmt.Printf("  Expires:     %s\n", expiry.Format("2006-01-02"))
			fmt.Println()
			fmt.Println("  Enable in config.yaml:")
			fmt.Println("    webui:")
			fmt.Println("      tls:")
			fmt.Println("        enabled: true")
			fmt.Println()

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "output directory (default: data/tls)")
	return cmd
}

// newTLSInfoCmd creates the `devclaw tls info` subcommand.
func newTLSInfoCmd() *cobra.Command {
	var certPath string

	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show TLS certificate information",
		Long: `Display the SHA-256 fingerprint and expiration date of the TLS certificate.

Examples:
  devclaw tls info
  devclaw tls info --cert /path/to/cert.pem`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if certPath == "" {
				certPath = filepath.Join("data", "tls", "devclaw-cert.pem")
			}

			if _, err := os.Stat(certPath); os.IsNotExist(err) {
				return fmt.Errorf("certificate not found: %s\nRun 'devclaw tls generate' to create one", certPath)
			}

			fp, err := devtls.CertFingerprint(certPath)
			if err != nil {
				return fmt.Errorf("failed to read fingerprint: %w", err)
			}

			expiry, err := devtls.CertExpiry(certPath)
			if err != nil {
				return fmt.Errorf("failed to read expiry: %w", err)
			}

			fmt.Printf("Certificate: %s\n", certPath)
			fmt.Printf("Fingerprint: SHA256:%s\n", fp)
			fmt.Printf("Expires:     %s\n", expiry.Format("2006-01-02 15:04:05 MST"))

			return nil
		},
	}

	cmd.Flags().StringVar(&certPath, "cert", "", "path to certificate PEM (default: data/tls/devclaw-cert.pem)")
	return cmd
}
