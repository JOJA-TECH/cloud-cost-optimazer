package adapters

import (
	"context"

	azpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

// tokenFunc acquires a bearer token for given credentials. Tests override it to
// avoid calling Microsoft Entra ID; production uses the real azidentity flow.
type tokenFunc func(ctx context.Context, creds cloud.ConnectionCredentials) (string, error)

// realToken acquires an ARM bearer token via azidentity client-secret flow.
// It never logs or returns the client secret.
func realToken(ctx context.Context, creds cloud.ConnectionCredentials) (string, error) {
	cred, err := azidentity.NewClientSecretCredential(creds.TenantID, creds.ClientID, creds.ClientSecret, nil)
	if err != nil {
		return "", err
	}
	tok, err := cred.GetToken(ctx, azpolicy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}
