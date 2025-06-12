package controller_test

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ErroringFakeClient struct {
	client.Client
	shouldErrorOnGet    bool
	shouldErrorOnUpdate bool
	shouldErrorOnCreate bool
	shouldErrorOnDelete bool
	errorOnNames        []types.NamespacedName
}

func (p *ErroringFakeClient) Get(
	ctx context.Context,
	key types.NamespacedName,
	acc client.Object,
	opts ...client.GetOption) error {

	if p.shouldErrorOnGet {
		nameToFail := false
		for _, x := range p.errorOnNames {
			if x.Namespace == key.Namespace && x.Name == key.Name {
				nameToFail = true
				break
			}
		}

		if nameToFail {
			return fmt.Errorf("Error getting")
		}
	}
	return p.Client.Get(ctx, key, acc, opts...)
}

func (p *ErroringFakeClient) Update(
	ctx context.Context,
	acc client.Object,
	opts ...client.UpdateOption) error {
	if p.shouldErrorOnUpdate {
		nameToFail := false
		for _, x := range p.errorOnNames {
			if x.Namespace == acc.GetNamespace() && x.Name == acc.GetName() {
				nameToFail = true
				break
			}
		}

		if nameToFail {
			return fmt.Errorf("Error updating")
		}
	}
	return p.Client.Update(ctx, acc, opts...)
}

func (p *ErroringFakeClient) Create(
	ctx context.Context,
	acc client.Object,
	opts ...client.CreateOption) error {
	if p.shouldErrorOnCreate {
		nameToFail := false
		for _, x := range p.errorOnNames {
			if x.Namespace == acc.GetNamespace() && x.Name == acc.GetName() {
				nameToFail = true
				break
			}
		}

		if nameToFail {
			return fmt.Errorf("Error creating")
		}
	}
	return p.Client.Create(ctx, acc, opts...)
}

func (p *ErroringFakeClient) Delete(
	ctx context.Context,
	acc client.Object,
	opts ...client.DeleteOption) error {
	if p.shouldErrorOnDelete {

		nameToFail := false
		for _, x := range p.errorOnNames {
			if x.Namespace == acc.GetNamespace() && x.Name == acc.GetName() {
				nameToFail = true
				break
			}
		}

		if nameToFail {
			return fmt.Errorf("Error deleting")
		}
	}
	return p.Client.Delete(ctx, acc, opts...)
}
