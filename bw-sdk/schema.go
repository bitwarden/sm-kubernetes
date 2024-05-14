package sdk

type EncString string

// Basic client behavior settings. These settings specify the various targets and behavior
// of the Bitwarden Client. They are optional and uneditable once the client is
// initialized.
//
// Defaults to
//
// ``` # use bitwarden::client::client_settings::{ClientSettings, DeviceType}; let settings
// = ClientSettings { identity_url: "https://identity.bitwarden.com".to_string(), api_url:
// "https://api.bitwarden.com".to_string(), user_agent: "Bitwarden Rust-SDK".to_string(),
// device_type: DeviceType::SDK, }; let default = ClientSettings::default(); ```
type ClientSettings struct {
	// The api url of the targeted Bitwarden instance. Defaults to `https://api.bitwarden.com`            
	APIURL                                                                                    *string     `json:"apiUrl,omitempty"`
	// Device type to send to Bitwarden. Defaults to SDK                                                  
	DeviceType                                                                                *DeviceType `json:"deviceType,omitempty"`
	// The identity url of the targeted Bitwarden instance. Defaults to                                   
	// `https://identity.bitwarden.com`                                                                   
	IdentityURL                                                                               *string     `json:"identityUrl,omitempty"`
	// The user_agent to sent to Bitwarden. Defaults to `Bitwarden Rust-SDK`                              
	UserAgent                                                                                 *string     `json:"userAgent,omitempty"`
}

// Login with username and password
//
// This command is for initiating an authentication handshake with Bitwarden. Authorization
// may fail due to requiring 2fa or captcha challenge completion despite accurate
// credentials.
//
// This command is not capable of handling authentication requiring 2fa or captcha.
//
// Returns: [PasswordLoginResponse](bitwarden::auth::login::PasswordLoginResponse)
//
// Login with API Key
//
// This command is for initiating an authentication handshake with Bitwarden.
//
// Returns: [ApiKeyLoginResponse](bitwarden::auth::login::ApiKeyLoginResponse)
//
// Login with Secrets Manager Access Token
//
// This command is for initiating an authentication handshake with Bitwarden.
//
// Returns: [ApiKeyLoginResponse](bitwarden::auth::login::ApiKeyLoginResponse)
//
// > Requires Authentication Get the API key of the currently authenticated user
//
// Returns: [UserApiKeyResponse](bitwarden::platform::UserApiKeyResponse)
//
// Get the user's passphrase
//
// Returns: String
//
// > Requires Authentication Retrieve all user data, ciphers and organizations the user is a
// part of
//
// Returns: [SyncResponse](bitwarden::platform::SyncResponse)
type Command struct {
	PasswordLogin    *PasswordLoginRequest      `json:"passwordLogin,omitempty"`
	APIKeyLogin      *APIKeyLoginRequest        `json:"apiKeyLogin,omitempty"`
	AccessTokenLogin *AccessTokenLoginRequest   `json:"accessTokenLogin,omitempty"`
	GetUserAPIKey    *SecretVerificationRequest `json:"getUserApiKey,omitempty"`
	Fingerprint      *FingerprintRequest        `json:"fingerprint,omitempty"`
	Sync             *SyncRequest               `json:"sync,omitempty"`
	Secrets          *SecretsCommand            `json:"secrets,omitempty"`
	Projects         *ProjectsCommand           `json:"projects,omitempty"`
}

// Login to Bitwarden with Api Key
type APIKeyLoginRequest struct {
	// Bitwarden account client_id             
	ClientID                            string `json:"clientId"`
	// Bitwarden account client_secret         
	ClientSecret                        string `json:"clientSecret"`
	// Bitwarden account master password       
	Password                            string `json:"password"`
}

// Login to Bitwarden with access token
type AccessTokenLoginRequest struct {
	// Bitwarden service API access token        
	AccessToken                          string  `json:"accessToken"`
	StateFile                            *string `json:"stateFile,omitempty"`
}

type FingerprintRequest struct {
	// The input material, used in the fingerprint generation process.       
	FingerprintMaterial                                               string `json:"fingerprintMaterial"`
	// The user's public key encoded with base64.                            
	PublicKey                                                         string `json:"publicKey"`
}

type SecretVerificationRequest struct {
	// The user's master password to use for user verification. If supplied, this will be used        
	// for verification purposes.                                                                     
	MasterPassword                                                                            *string `json:"masterPassword,omitempty"`
	// Alternate user verification method through OTP. This is provided for users who have no         
	// master password due to use of Customer Managed Encryption. Must be present and valid if        
	// master_password is absent.                                                                     
	Otp                                                                                       *string `json:"otp,omitempty"`
}

// Login to Bitwarden with Username and Password
type PasswordLoginRequest struct {
	// Bitwarden account email address                    
	Email                               string            `json:"email"`
	// Kdf from prelogin                                  
	Kdf                                 Kdf               `json:"kdf"`
	// Bitwarden account master password                  
	Password                            string            `json:"password"`
	TwoFactor                           *TwoFactorRequest `json:"twoFactor,omitempty"`
}

// Kdf from prelogin
type Kdf struct {
	PBKDF2   *PBKDF2   `json:"pBKDF2,omitempty"`
	Argon2ID *Argon2ID `json:"argon2id,omitempty"`
}

type Argon2ID struct {
	Iterations  int64 `json:"iterations"`
	Memory      int64 `json:"memory"`
	Parallelism int64 `json:"parallelism"`
}

type PBKDF2 struct {
	Iterations int64 `json:"iterations"`
}

type TwoFactorRequest struct {
	// Two-factor provider                  
	Provider              TwoFactorProvider `json:"provider"`
	// Two-factor remember                  
	Remember              bool              `json:"remember"`
	// Two-factor Token                     
	Token                 string            `json:"token"`
}

// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Retrieve a project by the provided identifier
//
// Returns: [ProjectResponse](bitwarden::secrets_manager::projects::ProjectResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Creates a new project in the provided organization using the given data
//
// Returns: [ProjectResponse](bitwarden::secrets_manager::projects::ProjectResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Lists all projects of the given organization
//
// Returns: [ProjectsResponse](bitwarden::secrets_manager::projects::ProjectsResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Updates an existing project with the provided ID using the given data
//
// Returns: [ProjectResponse](bitwarden::secrets_manager::projects::ProjectResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Deletes all the projects whose IDs match the provided ones
//
// Returns:
// [ProjectsDeleteResponse](bitwarden::secrets_manager::projects::ProjectsDeleteResponse)
type ProjectsCommand struct {
	Get    *ProjectGetRequest     `json:"get,omitempty"`
	Create *ProjectCreateRequest  `json:"create,omitempty"`
	List   *ProjectsListRequest   `json:"list,omitempty"`
	Update *ProjectPutRequest     `json:"update,omitempty"`
	Delete *ProjectsDeleteRequest `json:"delete,omitempty"`
}

type ProjectCreateRequest struct {
	Name                                             string `json:"name"`
	// Organization where the project will be created       
	OrganizationID                                   string `json:"organizationId"`
}

type ProjectsDeleteRequest struct {
	// IDs of the projects to delete         
	IDS                             []string `json:"ids"`
}

type ProjectGetRequest struct {
	// ID of the project to retrieve       
	ID                              string `json:"id"`
}

type ProjectsListRequest struct {
	// Organization to retrieve all the projects from       
	OrganizationID                                   string `json:"organizationId"`
}

type ProjectPutRequest struct {
	// ID of the project to modify                    
	ID                                         string `json:"id"`
	Name                                       string `json:"name"`
	// Organization ID of the project to modify       
	OrganizationID                             string `json:"organizationId"`
}

// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Retrieve a secret by the provided identifier
//
// Returns: [SecretResponse](bitwarden::secrets_manager::secrets::SecretResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Retrieve secrets by the provided identifiers
//
// Returns: [SecretsResponse](bitwarden::secrets_manager::secrets::SecretsResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Creates a new secret in the provided organization using the given data
//
// Returns: [SecretResponse](bitwarden::secrets_manager::secrets::SecretResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Lists all secret identifiers of the given organization, to then retrieve each
// secret, use `CreateSecret`
//
// Returns:
// [SecretIdentifiersResponse](bitwarden::secrets_manager::secrets::SecretIdentifiersResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Updates an existing secret with the provided ID using the given data
//
// Returns: [SecretResponse](bitwarden::secrets_manager::secrets::SecretResponse)
//
// > Requires Authentication > Requires using an Access Token for login or calling Sync at
// least once Deletes all the secrets whose IDs match the provided ones
//
// Returns:
// [SecretsDeleteResponse](bitwarden::secrets_manager::secrets::SecretsDeleteResponse)
type SecretsCommand struct {
	Get      *SecretGetRequest         `json:"get,omitempty"`
	GetByIDS *SecretsGetRequest        `json:"getByIds,omitempty"`
	Create   *SecretCreateRequest      `json:"create,omitempty"`
	List     *SecretIdentifiersRequest `json:"list,omitempty"`
	Update   *SecretPutRequest         `json:"update,omitempty"`
	Delete   *SecretsDeleteRequest     `json:"delete,omitempty"`
}

type SecretCreateRequest struct {
	Key                                                   string   `json:"key"`
	Note                                                  string   `json:"note"`
	// Organization where the secret will be created               
	OrganizationID                                        string   `json:"organizationId"`
	// IDs of the projects that this secret will belong to         
	ProjectIDS                                            []string `json:"projectIds,omitempty"`
	Value                                                 string   `json:"value"`
}

type SecretsDeleteRequest struct {
	// IDs of the secrets to delete         
	IDS                            []string `json:"ids"`
}

type SecretGetRequest struct {
	// ID of the secret to retrieve       
	ID                             string `json:"id"`
}

type SecretsGetRequest struct {
	// IDs of the secrets to retrieve         
	IDS                              []string `json:"ids"`
}

type SecretIdentifiersRequest struct {
	// Organization to retrieve all the secrets from       
	OrganizationID                                  string `json:"organizationId"`
}

type SecretPutRequest struct {
	// ID of the secret to modify                      
	ID                                        string   `json:"id"`
	Key                                       string   `json:"key"`
	Note                                      string   `json:"note"`
	// Organization ID of the secret to modify         
	OrganizationID                            string   `json:"organizationId"`
	ProjectIDS                                []string `json:"projectIds,omitempty"`
	Value                                     string   `json:"value"`
}

type SyncRequest struct {
	// Exclude the subdomains from the response, defaults to false      
	ExcludeSubdomains                                             *bool `json:"excludeSubdomains,omitempty"`
}

type ResponseForAPIKeyLoginResponse struct {
	// The response data. Populated if `success` is true.                                           
	Data                                                                       *APIKeyLoginResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                     
	ErrorMessage                                                               *string              `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                    
	Success                                                                    bool                 `json:"success"`
}

type APIKeyLoginResponse struct {
	Authenticated                                                         bool                `json:"authenticated"`
	// Whether or not the user is required to update their master password                    
	ForcePasswordReset                                                    bool                `json:"forcePasswordReset"`
	// TODO: What does this do?                                                               
	ResetMasterPassword                                                   bool                `json:"resetMasterPassword"`
	TwoFactor                                                             *TwoFactorProviders `json:"twoFactor,omitempty"`
}

type TwoFactorProviders struct {
	Authenticator                                                         *Authenticator `json:"authenticator,omitempty"`
	// Duo-backed 2fa                                                                    
	Duo                                                                   *Duo           `json:"duo,omitempty"`
	// Email 2fa                                                                         
	Email                                                                 *Email         `json:"email,omitempty"`
	// Duo-backed 2fa operated by an organization the user is a member of                
	OrganizationDuo                                                       *Duo           `json:"organizationDuo,omitempty"`
	// Presence indicates the user has stored this device as bypassing 2fa               
	Remember                                                              *Remember      `json:"remember,omitempty"`
	// WebAuthn-backed 2fa                                                               
	WebAuthn                                                              *WebAuthn      `json:"webAuthn,omitempty"`
	// Yubikey-backed 2fa                                                                
	YubiKey                                                               *YubiKey       `json:"yubiKey,omitempty"`
}

type Authenticator struct {
}

type Duo struct {
	Host      string `json:"host"`
	Signature string `json:"signature"`
}

type Email struct {
	// The email to request a 2fa TOTP for       
	Email                                 string `json:"email"`
}

type Remember struct {
}

type WebAuthn struct {
}

type YubiKey struct {
	// Whether the stored yubikey supports near field communication     
	NFC                                                            bool `json:"nfc"`
}

type ResponseForPasswordLoginResponse struct {
	// The response data. Populated if `success` is true.                                             
	Data                                                                       *PasswordLoginResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                       
	ErrorMessage                                                               *string                `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                      
	Success                                                                    bool                   `json:"success"`
}

type PasswordLoginResponse struct {
	Authenticated                                                                              bool                `json:"authenticated"`
	// The information required to present the user with a captcha challenge. Only present when                    
	// authentication fails due to requiring validation of a captcha challenge.                                    
	CAPTCHA                                                                                    *CAPTCHAResponse    `json:"captcha,omitempty"`
	// Whether or not the user is required to update their master password                                         
	ForcePasswordReset                                                                         bool                `json:"forcePasswordReset"`
	// TODO: What does this do?                                                                                    
	ResetMasterPassword                                                                        bool                `json:"resetMasterPassword"`
	// The available two factor authentication options. Present only when authentication fails                     
	// due to requiring a second authentication factor.                                                            
	TwoFactor                                                                                  *TwoFactorProviders `json:"twoFactor,omitempty"`
}

type CAPTCHAResponse struct {
	// hcaptcha site key       
	SiteKey             string `json:"siteKey"`
}

type ResponseForAccessTokenLoginResponse struct {
	// The response data. Populated if `success` is true.                                                
	Data                                                                       *AccessTokenLoginResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                          
	ErrorMessage                                                               *string                   `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                         
	Success                                                                    bool                      `json:"success"`
}

type AccessTokenLoginResponse struct {
	Authenticated                                                         bool                `json:"authenticated"`
	// Whether or not the user is required to update their master password                    
	ForcePasswordReset                                                    bool                `json:"forcePasswordReset"`
	// TODO: What does this do?                                                               
	ResetMasterPassword                                                   bool                `json:"resetMasterPassword"`
	TwoFactor                                                             *TwoFactorProviders `json:"twoFactor,omitempty"`
}

type ResponseForSecretIdentifiersResponse struct {
	// The response data. Populated if `success` is true.                                                 
	Data                                                                       *SecretIdentifiersResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                           
	ErrorMessage                                                               *string                    `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                          
	Success                                                                    bool                       `json:"success"`
}

type SecretIdentifiersResponse struct {
	Data []SecretIdentifierResponse `json:"data"`
}

type SecretIdentifierResponse struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	OrganizationID string `json:"organizationId"`
}

type ResponseForSecretResponse struct {
	// The response data. Populated if `success` is true.                                      
	Data                                                                       *SecretResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                
	ErrorMessage                                                               *string         `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                               
	Success                                                                    bool            `json:"success"`
}

type SecretResponse struct {
	CreationDate   string  `json:"creationDate"`
	ID             string  `json:"id"`
	Key            string  `json:"key"`
	Note           string  `json:"note"`
	OrganizationID string  `json:"organizationId"`
	ProjectID      *string `json:"projectId,omitempty"`
	RevisionDate   string  `json:"revisionDate"`
	Value          string  `json:"value"`
}

type ResponseForSecretsResponse struct {
	// The response data. Populated if `success` is true.                                       
	Data                                                                       *SecretsResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                 
	ErrorMessage                                                               *string          `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                
	Success                                                                    bool             `json:"success"`
}

type SecretsResponse struct {
	Data []SecretResponse `json:"data"`
}

type ResponseForSecretsDeleteResponse struct {
	// The response data. Populated if `success` is true.                                             
	Data                                                                       *SecretsDeleteResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                       
	ErrorMessage                                                               *string                `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                      
	Success                                                                    bool                   `json:"success"`
}

type SecretsDeleteResponse struct {
	Data []SecretDeleteResponse `json:"data"`
}

type SecretDeleteResponse struct {
	Error *string `json:"error,omitempty"`
	ID    string  `json:"id"`
}

type ResponseForProjectResponse struct {
	// The response data. Populated if `success` is true.                                       
	Data                                                                       *ProjectResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                 
	ErrorMessage                                                               *string          `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                
	Success                                                                    bool             `json:"success"`
}

type ProjectResponse struct {
	CreationDate   string `json:"creationDate"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	OrganizationID string `json:"organizationId"`
	RevisionDate   string `json:"revisionDate"`
}

type ResponseForProjectsResponse struct {
	// The response data. Populated if `success` is true.                                        
	Data                                                                       *ProjectsResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                  
	ErrorMessage                                                               *string           `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                 
	Success                                                                    bool              `json:"success"`
}

type ProjectsResponse struct {
	Data []ProjectResponse `json:"data"`
}

type ResponseForProjectsDeleteResponse struct {
	// The response data. Populated if `success` is true.                                              
	Data                                                                       *ProjectsDeleteResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                        
	ErrorMessage                                                               *string                 `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                       
	Success                                                                    bool                    `json:"success"`
}

type ProjectsDeleteResponse struct {
	Data []ProjectDeleteResponse `json:"data"`
}

type ProjectDeleteResponse struct {
	Error *string `json:"error,omitempty"`
	ID    string  `json:"id"`
}

type ResponseForFingerprintResponse struct {
	// The response data. Populated if `success` is true.                                           
	Data                                                                       *FingerprintResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                     
	ErrorMessage                                                               *string              `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                    
	Success                                                                    bool                 `json:"success"`
}

type FingerprintResponse struct {
	Fingerprint string `json:"fingerprint"`
}

type ResponseForSyncResponse struct {
	// The response data. Populated if `success` is true.                                    
	Data                                                                       *SyncResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.              
	ErrorMessage                                                               *string       `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                             
	Success                                                                    bool          `json:"success"`
}

type SyncResponse struct {
	// List of ciphers accessible by the user                                                               
	Ciphers                                                                                 []Cipher        `json:"ciphers"`
	Collections                                                                             []Collection    `json:"collections"`
	Domains                                                                                 *DomainResponse `json:"domains,omitempty"`
	Folders                                                                                 []Folder        `json:"folders"`
	Policies                                                                                []Policy        `json:"policies"`
	// Data about the user, including their encryption keys and the organizations they are a                
	// part of                                                                                              
	Profile                                                                                 ProfileResponse `json:"profile"`
	Sends                                                                                   []Send          `json:"sends"`
}

type Cipher struct {
	Attachments                                                                              []Attachment       `json:"attachments,omitempty"`
	Card                                                                                     *Card              `json:"card,omitempty"`
	CollectionIDS                                                                            []string           `json:"collectionIds"`
	CreationDate                                                                             string             `json:"creationDate"`
	DeletedDate                                                                              *string            `json:"deletedDate,omitempty"`
	Edit                                                                                     bool               `json:"edit"`
	Favorite                                                                                 bool               `json:"favorite"`
	Fields                                                                                   []Field            `json:"fields,omitempty"`
	FolderID                                                                                 *string            `json:"folderId,omitempty"`
	ID                                                                                       *string            `json:"id,omitempty"`
	Identity                                                                                 *Identity          `json:"identity,omitempty"`
	// More recent ciphers uses individual encryption keys to encrypt the other fields of the                   
	// Cipher.                                                                                                  
	Key                                                                                      *string            `json:"key,omitempty"`
	LocalData                                                                                *LocalData         `json:"localData,omitempty"`
	Login                                                                                    *Login             `json:"login,omitempty"`
	Name                                                                                     string             `json:"name"`
	Notes                                                                                    *string            `json:"notes,omitempty"`
	OrganizationID                                                                           *string            `json:"organizationId,omitempty"`
	OrganizationUseTotp                                                                      bool               `json:"organizationUseTotp"`
	PasswordHistory                                                                          []PasswordHistory  `json:"passwordHistory,omitempty"`
	Reprompt                                                                                 CipherRepromptType `json:"reprompt"`
	RevisionDate                                                                             string             `json:"revisionDate"`
	SecureNote                                                                               *SecureNote        `json:"secureNote,omitempty"`
	Type                                                                                     CipherType         `json:"type"`
	ViewPassword                                                                             bool               `json:"viewPassword"`
}

type Attachment struct {
	FileName                                   *string `json:"fileName,omitempty"`
	ID                                         *string `json:"id,omitempty"`
	Key                                        *string `json:"key,omitempty"`
	Size                                       *string `json:"size,omitempty"`
	// Readable size, ex: "4.2 KB" or "1.43 GB"        
	SizeName                                   *string `json:"sizeName,omitempty"`
	URL                                        *string `json:"url,omitempty"`
}

type Card struct {
	Brand          *string `json:"brand,omitempty"`
	CardholderName *string `json:"cardholderName,omitempty"`
	Code           *string `json:"code,omitempty"`
	ExpMonth       *string `json:"expMonth,omitempty"`
	ExpYear        *string `json:"expYear,omitempty"`
	Number         *string `json:"number,omitempty"`
}

type Field struct {
	LinkedID *LinkedIDType `json:"linkedId,omitempty"`
	Name     *string       `json:"name,omitempty"`
	Type     FieldType     `json:"type"`
	Value    *string       `json:"value,omitempty"`
}

type Identity struct {
	Address1       *string `json:"address1,omitempty"`
	Address2       *string `json:"address2,omitempty"`
	Address3       *string `json:"address3,omitempty"`
	City           *string `json:"city,omitempty"`
	Company        *string `json:"company,omitempty"`
	Country        *string `json:"country,omitempty"`
	Email          *string `json:"email,omitempty"`
	FirstName      *string `json:"firstName,omitempty"`
	LastName       *string `json:"lastName,omitempty"`
	LicenseNumber  *string `json:"licenseNumber,omitempty"`
	MiddleName     *string `json:"middleName,omitempty"`
	PassportNumber *string `json:"passportNumber,omitempty"`
	Phone          *string `json:"phone,omitempty"`
	PostalCode     *string `json:"postalCode,omitempty"`
	Ssn            *string `json:"ssn,omitempty"`
	State          *string `json:"state,omitempty"`
	Title          *string `json:"title,omitempty"`
	Username       *string `json:"username,omitempty"`
}

type LocalData struct {
	LastLaunched *int64 `json:"lastLaunched,omitempty"`
	LastUsedDate *int64 `json:"lastUsedDate,omitempty"`
}

type Login struct {
	AutofillOnPageLoad   *bool      `json:"autofillOnPageLoad,omitempty"`
	Password             *string    `json:"password,omitempty"`
	PasswordRevisionDate *string    `json:"passwordRevisionDate,omitempty"`
	Totp                 *string    `json:"totp,omitempty"`
	Uris                 []LoginURI `json:"uris,omitempty"`
	Username             *string    `json:"username,omitempty"`
}

type LoginURI struct {
	Match *URIMatchType `json:"match,omitempty"`
	URI   *string       `json:"uri,omitempty"`
}

type PasswordHistory struct {
	LastUsedDate string `json:"lastUsedDate"`
	Password     string `json:"password"`
}

type SecureNote struct {
	Type SecureNoteType `json:"type"`
}

type Collection struct {
	ExternalID     *string `json:"externalId,omitempty"`
	HidePasswords  bool    `json:"hidePasswords"`
	ID             *string `json:"id,omitempty"`
	Name           string  `json:"name"`
	OrganizationID string  `json:"organizationId"`
	ReadOnly       bool    `json:"readOnly"`
}

type DomainResponse struct {
	EquivalentDomains       [][]string      `json:"equivalentDomains"`
	GlobalEquivalentDomains []GlobalDomains `json:"globalEquivalentDomains"`
}

type GlobalDomains struct {
	Domains  []string `json:"domains"`
	Excluded bool     `json:"excluded"`
	Type     int64    `json:"type"`
}

type Folder struct {
	ID           *string `json:"id,omitempty"`
	Name         string  `json:"name"`
	RevisionDate string  `json:"revisionDate"`
}

type Policy struct {
	Data           map[string]interface{} `json:"data,omitempty"`
	Enabled        bool                   `json:"enabled"`
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	Type           PolicyType             `json:"type"`
}

// Data about the user, including their encryption keys and the organizations they are a
// part of
type ProfileResponse struct {
	Email         string                        `json:"email"`
	ID            string                        `json:"id"`
	Name          string                        `json:"name"`
	Organizations []ProfileOrganizationResponse `json:"organizations"`
}

type ProfileOrganizationResponse struct {
	ID string `json:"id"`
}

type Send struct {
	AccessCount    int64     `json:"accessCount"`
	AccessID       *string   `json:"accessId,omitempty"`
	DeletionDate   string    `json:"deletionDate"`
	Disabled       bool      `json:"disabled"`
	ExpirationDate *string   `json:"expirationDate,omitempty"`
	File           *SendFile `json:"file,omitempty"`
	HideEmail      bool      `json:"hideEmail"`
	ID             *string   `json:"id,omitempty"`
	Key            string    `json:"key"`
	MaxAccessCount *int64    `json:"maxAccessCount,omitempty"`
	Name           string    `json:"name"`
	Notes          *string   `json:"notes,omitempty"`
	Password       *string   `json:"password,omitempty"`
	RevisionDate   string    `json:"revisionDate"`
	Text           *SendText `json:"text,omitempty"`
	Type           SendType  `json:"type"`
}

type SendFile struct {
	FileName                                   string  `json:"fileName"`
	ID                                         *string `json:"id,omitempty"`
	Size                                       *string `json:"size,omitempty"`
	// Readable size, ex: "4.2 KB" or "1.43 GB"        
	SizeName                                   *string `json:"sizeName,omitempty"`
}

type SendText struct {
	Hidden bool    `json:"hidden"`
	Text   *string `json:"text,omitempty"`
}

type ResponseForUserAPIKeyResponse struct {
	// The response data. Populated if `success` is true.                                          
	Data                                                                       *UserAPIKeyResponse `json:"data,omitempty"`
	// A message for any error that may occur. Populated if `success` is false.                    
	ErrorMessage                                                               *string             `json:"errorMessage,omitempty"`
	// Whether or not the SDK request succeeded.                                                   
	Success                                                                    bool                `json:"success"`
}

type UserAPIKeyResponse struct {
	// The user's API key, which represents the client_secret portion of an oauth request.       
	APIKey                                                                                string `json:"apiKey"`
}

// Device type to send to Bitwarden. Defaults to SDK
type DeviceType string

const (
	Android          DeviceType = "Android"
	AndroidAmazon    DeviceType = "AndroidAmazon"
	ChromeBrowser    DeviceType = "ChromeBrowser"
	ChromeExtension  DeviceType = "ChromeExtension"
	EdgeBrowser      DeviceType = "EdgeBrowser"
	EdgeExtension    DeviceType = "EdgeExtension"
	FirefoxBrowser   DeviceType = "FirefoxBrowser"
	FirefoxExtension DeviceType = "FirefoxExtension"
	IEBrowser        DeviceType = "IEBrowser"
	IOS              DeviceType = "iOS"
	LinuxDesktop     DeviceType = "LinuxDesktop"
	MACOSDesktop     DeviceType = "MacOsDesktop"
	OperaBrowser     DeviceType = "OperaBrowser"
	OperaExtension   DeviceType = "OperaExtension"
	SDK              DeviceType = "SDK"
	SafariBrowser    DeviceType = "SafariBrowser"
	SafariExtension  DeviceType = "SafariExtension"
	UWP              DeviceType = "UWP"
	UnknownBrowser   DeviceType = "UnknownBrowser"
	VivaldiBrowser   DeviceType = "VivaldiBrowser"
	VivaldiExtension DeviceType = "VivaldiExtension"
	WindowsDesktop   DeviceType = "WindowsDesktop"
)

// Two-factor provider
type TwoFactorProvider string

const (
	OrganizationDuo                TwoFactorProvider = "OrganizationDuo"
	TwoFactorProviderAuthenticator TwoFactorProvider = "Authenticator"
	TwoFactorProviderDuo           TwoFactorProvider = "Duo"
	TwoFactorProviderEmail         TwoFactorProvider = "Email"
	TwoFactorProviderRemember      TwoFactorProvider = "Remember"
	TwoFactorProviderWebAuthn      TwoFactorProvider = "WebAuthn"
	U2F                            TwoFactorProvider = "U2f"
	Yubikey                        TwoFactorProvider = "Yubikey"
)

type LinkedIDType string

const (
	LinkedIDTypeAddress1       LinkedIDType = "Address1"
	LinkedIDTypeAddress2       LinkedIDType = "Address2"
	LinkedIDTypeAddress3       LinkedIDType = "Address3"
	LinkedIDTypeBrand          LinkedIDType = "Brand"
	LinkedIDTypeCardholderName LinkedIDType = "CardholderName"
	LinkedIDTypeCity           LinkedIDType = "City"
	LinkedIDTypeCode           LinkedIDType = "Code"
	LinkedIDTypeCompany        LinkedIDType = "Company"
	LinkedIDTypeCountry        LinkedIDType = "Country"
	LinkedIDTypeEmail          LinkedIDType = "Email"
	LinkedIDTypeExpMonth       LinkedIDType = "ExpMonth"
	LinkedIDTypeExpYear        LinkedIDType = "ExpYear"
	LinkedIDTypeFirstName      LinkedIDType = "FirstName"
	LinkedIDTypeFullName       LinkedIDType = "FullName"
	LinkedIDTypeLastName       LinkedIDType = "LastName"
	LinkedIDTypeLicenseNumber  LinkedIDType = "LicenseNumber"
	LinkedIDTypeMiddleName     LinkedIDType = "MiddleName"
	LinkedIDTypeNumber         LinkedIDType = "Number"
	LinkedIDTypePassportNumber LinkedIDType = "PassportNumber"
	LinkedIDTypePassword       LinkedIDType = "Password"
	LinkedIDTypePhone          LinkedIDType = "Phone"
	LinkedIDTypePostalCode     LinkedIDType = "PostalCode"
	LinkedIDTypeSsn            LinkedIDType = "Ssn"
	LinkedIDTypeState          LinkedIDType = "State"
	LinkedIDTypeTitle          LinkedIDType = "Title"
	LinkedIDTypeUsername       LinkedIDType = "Username"
)

type FieldType string

const (
	Boolean       FieldType = "Boolean"
	FieldTypeText FieldType = "Text"
	Hidden        FieldType = "Hidden"
	Linked        FieldType = "Linked"
)

type URIMatchType string

const (
	Domain            URIMatchType = "domain"
	Exact             URIMatchType = "exact"
	Host              URIMatchType = "host"
	Never             URIMatchType = "never"
	RegularExpression URIMatchType = "regularExpression"
	StartsWith        URIMatchType = "startsWith"
)

type CipherRepromptType string

const (
	CipherRepromptTypePassword CipherRepromptType = "Password"
	None                       CipherRepromptType = "None"
)

type SecureNoteType string

const (
	Generic SecureNoteType = "Generic"
)

type CipherType string

const (
	CipherTypeCard       CipherType = "Card"
	CipherTypeIdentity   CipherType = "Identity"
	CipherTypeLogin      CipherType = "Login"
	CipherTypeSecureNote CipherType = "SecureNote"
)

type PolicyType string

const (
	ActivateAutofill           PolicyType = "ActivateAutofill"
	DisablePersonalVaultExport PolicyType = "DisablePersonalVaultExport"
	DisableSend                PolicyType = "DisableSend"
	MasterPassword             PolicyType = "MasterPassword"
	MaximumVaultTimeout        PolicyType = "MaximumVaultTimeout"
	PasswordGenerator          PolicyType = "PasswordGenerator"
	PersonalOwnership          PolicyType = "PersonalOwnership"
	RequireSso                 PolicyType = "RequireSso"
	ResetPassword              PolicyType = "ResetPassword"
	SendOptions                PolicyType = "SendOptions"
	SingleOrg                  PolicyType = "SingleOrg"
	TwoFactorAuthentication    PolicyType = "TwoFactorAuthentication"
)

type SendType string

const (
	File         SendType = "File"
	SendTypeText SendType = "Text"
)

type LoginLinkedIDType string

const (
	LoginLinkedIDTypePassword LoginLinkedIDType = "Password"
	LoginLinkedIDTypeUsername LoginLinkedIDType = "Username"
)

type CardLinkedIDType string

const (
	CardLinkedIDTypeBrand          CardLinkedIDType = "Brand"
	CardLinkedIDTypeCardholderName CardLinkedIDType = "CardholderName"
	CardLinkedIDTypeCode           CardLinkedIDType = "Code"
	CardLinkedIDTypeExpMonth       CardLinkedIDType = "ExpMonth"
	CardLinkedIDTypeExpYear        CardLinkedIDType = "ExpYear"
	CardLinkedIDTypeNumber         CardLinkedIDType = "Number"
)

type IdentityLinkedIDType string

const (
	IdentityLinkedIDTypeAddress1       IdentityLinkedIDType = "Address1"
	IdentityLinkedIDTypeAddress2       IdentityLinkedIDType = "Address2"
	IdentityLinkedIDTypeAddress3       IdentityLinkedIDType = "Address3"
	IdentityLinkedIDTypeCity           IdentityLinkedIDType = "City"
	IdentityLinkedIDTypeCompany        IdentityLinkedIDType = "Company"
	IdentityLinkedIDTypeCountry        IdentityLinkedIDType = "Country"
	IdentityLinkedIDTypeEmail          IdentityLinkedIDType = "Email"
	IdentityLinkedIDTypeFirstName      IdentityLinkedIDType = "FirstName"
	IdentityLinkedIDTypeFullName       IdentityLinkedIDType = "FullName"
	IdentityLinkedIDTypeLastName       IdentityLinkedIDType = "LastName"
	IdentityLinkedIDTypeLicenseNumber  IdentityLinkedIDType = "LicenseNumber"
	IdentityLinkedIDTypeMiddleName     IdentityLinkedIDType = "MiddleName"
	IdentityLinkedIDTypePassportNumber IdentityLinkedIDType = "PassportNumber"
	IdentityLinkedIDTypePhone          IdentityLinkedIDType = "Phone"
	IdentityLinkedIDTypePostalCode     IdentityLinkedIDType = "PostalCode"
	IdentityLinkedIDTypeSsn            IdentityLinkedIDType = "Ssn"
	IdentityLinkedIDTypeState          IdentityLinkedIDType = "State"
	IdentityLinkedIDTypeTitle          IdentityLinkedIDType = "Title"
	IdentityLinkedIDTypeUsername       IdentityLinkedIDType = "Username"
)

