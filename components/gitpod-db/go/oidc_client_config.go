// Copyright (c) 2022 Gitpod GmbH. All rights reserved.
// Licensed under the GNU Affero General Public License (AGPL).
// See License.AGPL.txt in the project root for license information.

package db

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OIDCClientConfig struct {
	ID uuid.UUID `gorm:"primary_key;column:id;type:char;size:36;" json:"id"`

	OrganizationID uuid.UUID `gorm:"column:organizationId;type:char;size:36;" json:"organizationId"`

	Issuer string `gorm:"column:issuer;type:char;size:255;" json:"issuer"`

	Data EncryptedJSON[OIDCSpec] `gorm:"column:data;type:text;size:65535" json:"data"`

	Active bool `gorm:"column:active;type:tinyint;default:0;" json:"active"`

	LastModified time.Time `gorm:"column:_lastModified;type:timestamp;default:CURRENT_TIMESTAMP(6);" json:"_lastModified"`
	// deleted is reserved for use by periodic deleter.
	_ bool `gorm:"column:deleted;type:tinyint;default:0;" json:"deleted"`
}

func (c *OIDCClientConfig) TableName() string {
	return "d_b_oidc_client_config"
}

// It feels wrong to have to define re-define all of these fields.
// However, I could not find a Go library which would include json annotations on the structs to guarantee the fields
// will remain consistent over time (and resilient to rename). If we find one, we can change this.
type OIDCSpec struct {
	// ClientID is the application's ID.
	ClientID string `json:"clientId"`

	// ClientSecret is the application's secret.
	ClientSecret string `json:"clientSecret"`

	// RedirectURL is the URL to redirect users going through
	// the OAuth flow, after the resource owner's URLs.
	RedirectURL string `json:"redirectUrl"`

	// Scope specifies optional requested permissions.
	Scopes []string `json:"scopes"`
}

func CreateOIDCClientConfig(ctx context.Context, conn *gorm.DB, cfg OIDCClientConfig) (OIDCClientConfig, error) {
	if cfg.ID == uuid.Nil {
		return OIDCClientConfig{}, errors.New("id must be set")
	}

	if cfg.Issuer == "" {
		return OIDCClientConfig{}, errors.New("issuer must be set")
	}

	tx := conn.
		WithContext(ctx).
		Create(&cfg)
	if tx.Error != nil {
		return OIDCClientConfig{}, fmt.Errorf("failed to create oidc client config: %w", tx.Error)
	}

	return cfg, nil
}

func GetOIDCClientConfig(ctx context.Context, conn *gorm.DB, id uuid.UUID) (OIDCClientConfig, error) {
	var config OIDCClientConfig

	if id == uuid.Nil {
		return OIDCClientConfig{}, fmt.Errorf("OIDC Client Config ID is a required argument")
	}

	tx := conn.
		WithContext(ctx).
		Where("id = ?", id).
		Where("deleted = ?", 0).
		First(&config)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return OIDCClientConfig{}, fmt.Errorf("OIDC Client Config with ID %s does not exist: %w", id.String(), ErrorNotFound)
		}
		return OIDCClientConfig{}, fmt.Errorf("Failed to retrieve OIDC client config: %v", tx.Error)
	}

	return config, nil
}

func GetOIDCClientConfigForOrganization(ctx context.Context, conn *gorm.DB, id, organizationID uuid.UUID) (OIDCClientConfig, error) {
	var config OIDCClientConfig

	if id == uuid.Nil {
		return OIDCClientConfig{}, fmt.Errorf("OIDC Client Config ID is a required argument")
	}

	if organizationID == uuid.Nil {
		return OIDCClientConfig{}, fmt.Errorf("organization id is a required argument")
	}

	tx := conn.
		WithContext(ctx).
		Where("id = ?", id).
		Where("organizationId = ?", organizationID).
		Where("deleted = ?", 0).
		First(&config)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return OIDCClientConfig{}, fmt.Errorf("OIDC Client Config with ID %s for Organization ID %s does not exist: %w", id.String(), organizationID.String(), ErrorNotFound)
		}

		return OIDCClientConfig{}, fmt.Errorf("Failed to retrieve OIDC client config %s for Organization ID %s: %v", id.String(), organizationID.String(), tx.Error)
	}

	return config, nil
}

func ListOIDCClientConfigsForOrganization(ctx context.Context, conn *gorm.DB, organizationID uuid.UUID) ([]OIDCClientConfig, error) {
	if organizationID == uuid.Nil {
		return nil, errors.New("organization ID is a required argument")
	}

	var results []OIDCClientConfig

	tx := conn.
		WithContext(ctx).
		Where("organizationId = ?", organizationID.String()).
		Where("deleted = ?", 0).
		Order("id").
		Find(&results)
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to list oidc client configs for organization %s: %w", organizationID.String(), tx.Error)
	}

	return results, nil
}

func DeleteOIDCClientConfig(ctx context.Context, conn *gorm.DB, id, organizationID uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("id is a required argument")
	}

	if organizationID == uuid.Nil {
		return fmt.Errorf("organization id is a required argument")
	}

	tx := conn.
		WithContext(ctx).
		Table((&OIDCClientConfig{}).TableName()).
		Where("id = ?", id).
		Where("organizationId = ?", organizationID).
		Where("deleted = ?", 0).
		Update("deleted", 1)

	if tx.Error != nil {
		return fmt.Errorf("failed to delete oidc client config (ID: %s): %v", id.String(), tx.Error)
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("oidc client config ID: %s for organization ID: %s does not exist: %w", id.String(), organizationID.String(), ErrorNotFound)
	}

	return nil
}

func GetOIDCClientConfigByOrgSlug(ctx context.Context, conn *gorm.DB, slug string) (OIDCClientConfig, error) {
	var config OIDCClientConfig

	if slug == "" {
		return OIDCClientConfig{}, fmt.Errorf("slug is a required argument")
	}

	tx := conn.
		WithContext(ctx).
		Table((&OIDCClientConfig{}).TableName()).
		// TODO: is there a better way to reference table names here and below?
		Joins("JOIN d_b_team team ON team.id = d_b_oidc_client_config.organizationId").
		Where("team.slug = ?", slug).
		Where("d_b_oidc_client_config.deleted = ?", 0).
		First(&config)

	if tx.Error != nil {
		return OIDCClientConfig{}, fmt.Errorf("failed to get oidc client config by org slug (slug: %s): %v", slug, tx.Error)
	}

	return config, nil
}

// UpdateOIDCClientConfig performs an update of the OIDC Client config.
// Only non-zero fields specified in the struct are updated.
// When updating the encrypted contents of the specUpdate, you can specify them in the update to have re-encrypted in a transaction.
func UpdateOIDCClientConfig(ctx context.Context, conn *gorm.DB, cipher Cipher, update OIDCClientConfig, specUpdate *OIDCSpec) error {
	if update.ID == uuid.Nil {
		return errors.New("id is a required field")
	}

	txErr := conn.
		WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			if specUpdate != nil {
				// we also need to update the contents of the encrypted spec.
				existing, err := GetOIDCClientConfig(ctx, conn, update.ID)
				if err != nil {
					return err
				}

				decrypted, err := existing.Data.Decrypt(cipher)
				if err != nil {
					return fmt.Errorf("failed to decrypt oidc spec: %w", err)
				}

				updatedSpec := partialUpdateOIDCSpec(decrypted, *specUpdate)

				encrypted, err := EncryptJSON(cipher, updatedSpec)
				if err != nil {
					return fmt.Errorf("failed to encrypt oidc spec: %w", err)
				}

				// Set the serialized contents on our desired update object
				update.Data = encrypted
			}

			updateTx := tx.
				Model(&OIDCClientConfig{}).
				Where("id = ?", update.ID.String()).
				Where("deleted = ?", 0).
				Updates(update)
			if updateTx.Error != nil {
				return fmt.Errorf("failed to update OIDC client: %w", updateTx.Error)
			}

			if updateTx.RowsAffected == 0 {
				return fmt.Errorf("OIDC client config ID: %s does not exist: %w", update.ID.String(), ErrorNotFound)
			}

			// return nil will commit the whole transaction
			return nil
		})

	if txErr != nil {
		return fmt.Errorf("failed to update oidc spec ID: %s: %w", update.ID.String(), txErr)
	}

	return nil
}

func ActivateClientConfig(ctx context.Context, conn *gorm.DB, id uuid.UUID) error {
	return setClientConfigActiveFlag(ctx, conn, id, true)
}

func DeactivateClientConfig(ctx context.Context, conn *gorm.DB, id uuid.UUID) error {
	return setClientConfigActiveFlag(ctx, conn, id, false)
}

func setClientConfigActiveFlag(ctx context.Context, conn *gorm.DB, id uuid.UUID, active bool) error {
	_, err := GetOIDCClientConfig(ctx, conn, id)
	if err != nil {
		return err
	}

	value := 0
	if active {
		value = 1
	}

	tx := conn.
		WithContext(ctx).
		Table((&OIDCClientConfig{}).TableName()).
		Where("id = ?", id.String()).
		Update("active", value)
	if tx.Error != nil {
		return fmt.Errorf("failed to set oidc client config as active to %d (id: %s): %v", value, id.String(), tx.Error)
	}
	return nil
}

func partialUpdateOIDCSpec(old, new OIDCSpec) OIDCSpec {
	if new.ClientID != "" {
		old.ClientID = new.ClientID
	}

	if new.ClientSecret != "" {
		old.ClientSecret = new.ClientSecret
	}

	if new.RedirectURL != "" {
		old.RedirectURL = new.RedirectURL
	}

	if !oidcScopesEqual(old.Scopes, new.Scopes) {
		old.Scopes = new.Scopes
	}

	return old
}

func oidcScopesEqual(old, new []string) bool {
	if len(old) != len(new) {
		return false
	}

	sort.Strings(old)
	sort.Strings(new)

	for i := 0; i < len(old); i++ {
		if old[i] != new[i] {
			return false
		}
	}

	return true
}
