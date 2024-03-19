package controller

import (
	sdk "github.com/bitwarden/sdk/languages/go"
)

type BitwardenClientFactory interface {
	GetBitwardenClient() (sdk.BitwardenClientInterface, error)
	GetApiUrl() string
	GetIdentityApiUrl() string
}

// BitwardenSecretReconciler reconciles a BitwardenSecret object
type BitwardenClientFactoryImp struct {
	BwApiUrl    string
	IdentApiUrl string
}

func NewBitwardenClientFactory(bwApiUrl string, identApiUrl string) BitwardenClientFactory {
	return &BitwardenClientFactoryImp{
		BwApiUrl:    bwApiUrl,
		IdentApiUrl: identApiUrl,
	}
}

func (bc *BitwardenClientFactoryImp) GetBitwardenClient() (sdk.BitwardenClientInterface, error) {
	bitwardenClient, err := sdk.NewBitwardenClient(&bc.BwApiUrl, &bc.IdentApiUrl)
	if err != nil {
		return nil, err
	}

	return bitwardenClient, nil
}

func (bc *BitwardenClientFactoryImp) GetApiUrl() string {
	return bc.BwApiUrl
}

func (bc *BitwardenClientFactoryImp) GetIdentityApiUrl() string {
	return bc.IdentApiUrl
}
