package link

type CredentialsInfo struct {
	Type        string
	ClientID    string
	ProjectID   string
	Environment string
	APIBaseURL  string
}

func (c *Client) CredentialsInfo() CredentialsInfo {
	if c == nil || c.creds == nil {
		return CredentialsInfo{}
	}
	return CredentialsInfo{
		Type:        c.creds.Type,
		ClientID:    c.creds.ClientID,
		ProjectID:   c.creds.ProjectID,
		Environment: c.creds.Environment,
		APIBaseURL:  c.creds.APIBaseURL,
	}
}
