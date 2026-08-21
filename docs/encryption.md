# Encryption & Signed URLs

## Application key

Everything cryptographic is keyed by `APP_KEY`:

```bash
genesys key:generate          # writes APP_KEY=base64:... to .env
genesys key:generate --show   # print without writing
```

Register the provider so the facade is available:

```go
app.Register(&providers.EncryptionServiceProvider{})
```

## Encrypting values

AES-256-GCM authenticated encryption — tampered or truncated payloads are
rejected, never partially decrypted:

```go
import cryptfacade "github.com/genesysflow/go-genesys/facades/crypt"

payload, err := cryptfacade.EncryptString("secret value")
plain, err := cryptfacade.DecryptString(payload)
if errors.Is(err, crypt.ErrInvalidPayload) {
    // wrong key, tampering, or corruption
}
```

Direct use without the container:

```go
encrypter, err := crypt.NewFromString(os.Getenv("APP_KEY"))
```

## Signed URLs

Tamper-proof links (email verification, unsubscribe, downloads):

```go
key, _ := crypt.ParseKey(env.Require("APP_KEY"))
signer := urlsign.New(key)

link, err := signer.Sign("https://app.test/unsubscribe?user=42")
temp, err := signer.SignTemporary("https://app.test/download?f=1", 24*time.Hour)

ok := signer.Verify(incomingURL)
```

Protect routes with the middleware:

```go
kernel.GET("/unsubscribe", Unsubscribe, middleware.ValidateSignature(key))
// Invalid or expired signatures get 403 before the handler runs.
```

Signatures are HMAC-SHA256 over the path and canonically-ordered query
string, so parameter reordering does not break verification and parameter
tampering always does.
