package alks

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// LongTermKey represents a long term key
type LongTermKey struct {
	UserName    string `json:"userName"`
	AccessKeyID string `json:"accessKeyId"`
	Status      string `json:"status"`
	CreateDate  string `json:"createDate"`
}

// GetLongTermKeysResponse is used to represent the list of long term keys
type GetLongTermKeysResponse struct {
	BaseResponse
	LongTermKeys []LongTermKey `json:"longTermKeys"`
}

// CreateLongTermKey represents the response from API
type CreateLongTermKey struct {
	Account             string `json:"account"`
	Action              string `json:"action"`
	IAMUserName         string `json:"iamUserName"`
	IAMUserArn          string `json:"iamUserArn"`
	AddedIAMUserToGroup bool   `json:"addedIAMUserToGroup"`
	PartialError        bool   `json:"partialError"`
	AccessKey           string `json:"accessKey"`
	SecretKey           string `json:"secretKey"`
}

// LongTermKeyRequest is used to represent the request body to create or delete LTKs
type LongTermKeyRequest struct {
	Account     string `json:"account"`
	IamUserName string `json:"iamUserName"`
}

// CreateLongTermKeyResponse is the response to the CLI client
type CreateLongTermKeyResponse struct {
	BaseResponse
	CreateLongTermKey
}

// DeleteLongTermKey represents the response from API
type DeleteLongTermKey struct {
	AddedIAMUserToGroup bool `json:"addedIAMUserToGroup"`
	PartialError        bool `json:"partialError"`
}

// DeleteLongTermKeyResponse is the response to the CLI client
type DeleteLongTermKeyResponse struct {
	BaseResponse
	DeleteLongTermKey
}

// GetLongTermKeys gets the LTKs for an account
// If no error is returned then you will receive a list of LTKs
func (c *Client) GetLongTermKeys(accountId string, roleName string) (*GetLongTermKeysResponse, error) {
	log.Printf("[INFO] Getting long term keys for: %s/%s", accountId, roleName)

	req, err := c.NewRequest(nil, "GET", "/ltks/"+accountId+"/"+roleName)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	cr := new(GetLongTermKeysResponse)
	err = decodeBody(resp, &cr)

	if err != nil {
		if reqId := GetRequestID(resp); reqId != "" {
			return nil, fmt.Errorf("Error parsing GetLongTermKeysResponse: [%s] %s", reqId, err)
		}

		return nil, fmt.Errorf("Error parsing GetLongTermKeysResponse: %s", err)
	}

	if cr.RequestFailed() {
		return nil, fmt.Errorf("Error getting long term keys: [%s] %s", cr.BaseResponse.RequestID, strings.Join(cr.GetErrors(), ", "))
	}

	return cr, nil
}

// CreateLongTermKeys creates an LTK user for an account.
// If no error is returned, then you will receive an appropriate success message.
func (c *Client) CreateLongTermKey(accountId string, roleName string, accountAlias string, iamUsername string) (*CreateLongTermKeyResponse, error) {
	log.Printf("[INFO] Creating long term key for: %s/%s - %s", accountId, roleName, accountAlias)

	request := LongTermKeyRequest{
		Account:     accountId + "/ALKS" + roleName + " - " + accountAlias,
		IamUserName: iamUsername,
	}

	reqBody, err := json.Marshal(struct{ LongTermKeyRequest }{request})

	if err != nil {
		return nil, fmt.Errorf("error encoding LTK create JSON: %s", err)
	}

	req, err := c.NewRequest(reqBody, "POST", "/accessKeys")
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	cr := new(CreateLongTermKeyResponse)
	err = decodeBody(resp, &cr)

	if err != nil {
		if reqId := GetRequestID(resp); reqId != "" {
			return nil, fmt.Errorf("error parsing CreateLongTermKeyResponse: [%s] %s", reqId, err)
		}

		return nil, fmt.Errorf("error parsing CreateLongTermKeyResponse: %s", err)
	}

	if cr.RequestFailed() {
		return nil, fmt.Errorf("error creating long term key: [%s] %s", cr.BaseResponse.RequestID, strings.Join(cr.GetErrors(), ", "))
	}

	return cr, nil
}

// DeleteLongTermKeys deletes an LTK user for an account.
// If no error is returned, then you will receive an appropriate success message.
func (c *Client) DeleteLongTermKey(accountId string, roleName string, accountAlias string, iamUsername string) (*DeleteLongTermKeyResponse, error) {
	log.Printf("[INFO] Deleting long term key user for: %s/%s - %s", accountId, roleName, accountAlias)

	request := LongTermKeyRequest{
		Account:     accountId + "/ALKS" + roleName + " - " + accountAlias,
		IamUserName: iamUsername,
	}
	reqBody, err := json.Marshal(struct{ LongTermKeyRequest }{request})

	if err != nil {
		return nil, fmt.Errorf("error encoding LTK delete JSON: %s", err)
	}

	req, err := c.NewRequest(reqBody, "DELETE", "/IAMUser")
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	cr := new(DeleteLongTermKeyResponse)
	err = decodeBody(resp, &cr)

	if err != nil {
		if reqId := GetRequestID(resp); reqId != "" {
			return nil, fmt.Errorf("error parsing DeleteLongTermKeyResponse: [%s] %s", reqId, err)
		}

		return nil, fmt.Errorf("error parsing DeleteLongTermKeyResponse: %s", err)
	}

	if cr.RequestFailed() {
		return nil, fmt.Errorf("error deleting long term key: [%s] %s", cr.BaseResponse.RequestID, strings.Join(cr.GetErrors(), ", "))
	}

	return cr, nil

}
