package alks

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type IsIamRoleRequest struct {
	RoleArn string `json:"roleArn"`
}

// IsIamRoleResponse is used to represent a role that's IAM active or not.
type IsIamRoleResponse struct {
	BaseResponse
	RoleArn    string `json:"roleArn"`
	IamEnabled bool   `json:"iamEnabled"`
}

// IsIamEnabled will check if a MI or STS assumed role is IAM active or not.
func (c *Client) IsIamEnabled(roleArn string) (*IsIamRoleResponse, error) {
	log.Printf("[INFO] Is IAM enabled for MI: %s", roleArn)

	var iam IsIamRoleRequest
	var body []byte
	var err error

	if len(roleArn) > 1 {
		log.Printf("[INFO] Is IAM enabled for MI: %s", roleArn)

		// Request for a MI.
		iam = IsIamRoleRequest{
			roleArn,
		}

		body, err = json.Marshal(struct {
			IsIamRoleRequest
		}{iam})
	} else {
		log.Printf("[INFO] Is IAM enabled for STS: %s", c.AccountDetails.Role)

		// Request for STS
		body, err = json.Marshal(struct {}{})
	}

	if err != nil {
		return nil, fmt.Errorf("error encoding IAM create role JSON: %s", err)
	}

	req, err := c.NewRequest(body, "POST", "/isIamEnabled")
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	validate := new(IsIamRoleResponse)
	err = decodeBody(resp, validate)

	if err != nil {
		if reqID := GetRequestID(resp); reqID != "" {
			return nil, fmt.Errorf("error parsing isIamEnabled response: [%s] %s", reqID, err)
		}

		return nil, fmt.Errorf("error parsing isIamEnabled response: %s", err)
	}
	if validate.RequestFailed() {
		return nil, fmt.Errorf("error validating if MI is active: [%s] %s", validate.BaseResponse.RequestID, strings.Join(validate.GetErrors(), ", "))
	}

	return validate, nil
}
