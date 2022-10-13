package alks

import (
	"encoding/json"
	"fmt"
	"log"

	// "net/http"
	"strings"
)

type IamUser struct {
	ARN       string `json:"arn"`
	AccountId string `json:"accountId"`
	UserName  string `json:"userName"`
	AccessKey string `json:"accessKey"`
	Tags      []Tag  `json:"tags"`
}



// GetIamUsersResponse is used to represent the list of long term keys
// This type will be used for the new iam-user endpoint when it's created
// type GetIamUsersResponse struct {
// 	BaseResponse
// 	Users []IamUser `json:"items"`
// }

// GetIamUserResponse is used to represent a single long term key.
type GetIamUserResponse struct {
	BaseResponse
	User IamUser `json:"item"`
}

type CreateIamUserOptions struct {
	IamUserName *string
	Tags        *[]Tag
}


//These types will be used for the new iam-user endpoint when it's created. 
// type IamUserWithSecret struct {
// 	IamUser   IamUser
// 	SecretKey string `json:"secretKey"`
// }
//
// type CreateIamUserRequest struct {
// 	User struct {
// 		IamUserName string `json:"userName"`
// 		Tags        []Tag  `json:"tags,omitempty"`
// 	} `json:"user"`
// }
//
// // CreateIamUserResponse is the response to the CLI client
// type CreateIamUserResponse struct {
// 	BaseResponse
// 	User IamUserWithSecret `json:"items"`
// }

// DeleteIamUserRequest is used to represent the request body to delete LTKs
type DeleteIamUserRequest struct {
	AccountDetails
	IamUserName string `json:"iamUserName"`
}

type DeleteIamUserResponse struct {
	AccountDetails
	IamUserName string `json:"iamUserName"`
}

type UpdateIamUserRequest struct {
	IamUserName *string `json:"userName"`
	Tags        *[]Tag  `json:"tags"`
}

type UpdateIamUserResponse struct {
	User IamUser `json:"item"`
}

// GetIamUsers gets the LTKs for an account
// If no error is returned then you will receive a list of LTKs
func (c *Client) GetIamUsers() (*GetIamUsersResponse, *AlksError) {
	log.Printf("[INFO] Getting long term keys")

	accountID, err := c.AccountDetails.GetAccountNumber()
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        fmt.Errorf("Error reading Account value: %s", err),
		}
	}

	// roleName, err := c.AccountDetails.GetRoleName(false)
	// if err != nil {
	// 	return nil, &AlksError{
	// 		StatusCode: 0,
	// 		RequestId:  "",
	// 		Err: fmt.Errorf("Error reading Role value: %s", err),
	// 	}
	// }

	req, err := c.NewRequest(nil, "GET", "/iam-users/id/"+accountID)
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        err,
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        err,
		}
	}

	reqID := GetRequestID(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		keyErr := new(AlksResponseError)
		err = decodeBody(resp, &keyErr)
		if err != nil {
			return nil, &AlksError{
				StatusCode: resp.StatusCode,
				RequestId:  reqID,
				Err:        fmt.Errorf(ParseErrorReqId, reqID, err),
			}
		}

		if keyErr.Errors != nil {
			return nil, &AlksError{
				StatusCode: resp.StatusCode,
				RequestId:  reqID,
				Err:        fmt.Errorf(ErrorStringFull, reqID, resp.StatusCode, strings.Join(keyErr.Errors, ", ")),
			}
		}

		return nil, &AlksError{
			StatusCode: resp.StatusCode,
			RequestId:  reqID,
			Err:        fmt.Errorf(ErrorStringOnlyCodeAndReqId, reqID, resp.StatusCode),
		}
	}

	cr := new(GetIamUsersResponse)
	err = decodeBody(resp, &cr)

	if err != nil {
		return nil, &AlksError{
			StatusCode: resp.StatusCode,
			RequestId:  reqID,
			Err:        fmt.Errorf("Error parsing GetLongTermKeysResponse: [%s] %s", reqID, err),
		}
	}

	return cr, nil
}

// GetIamUser gets a single LTK for an account
// If no error is returned, then you will receive an LTK for the given account.
func (c *Client) GetIamUser(iamUsername string) (*GetIamUserResponse, error) {
	log.Printf("[INFO] Getting long term key")

	accountID, err := c.AccountDetails.GetAccountNumber()
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        fmt.Errorf("Error reading Account value: %s", err),
		}
	}

	req, err := c.NewRequest(nil, "GET", "/iam-users/id/"+accountID+"/"+iamUsername)

	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        fmt.Errorf("Error creating request object: %s", err),
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        fmt.Errorf("Error during request: %s", err),
		}
	}

	reqID := GetRequestID(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		keyErr := new(AlksResponseError)
		err = decodeBody(resp, &keyErr)
		if err != nil {
			return nil, &AlksError{
				StatusCode: resp.StatusCode,
				RequestId:  reqID,
				Err:        fmt.Errorf(ParseErrorReqId, reqID, err),
			}
		}

		if keyErr.Errors != nil {
			if reqID := GetRequestID(resp); reqID != "" {
				return nil, &AlksError{
					StatusCode: resp.StatusCode,
					RequestId:  reqID,
					Err:        fmt.Errorf(ErrorStringFull, reqID, resp.StatusCode, keyErr.Errors),
				}
			}
		}

		return nil, &AlksError{
			StatusCode: resp.StatusCode,
			RequestId:  reqID,
			Err:        fmt.Errorf(ErrorStringOnlyCodeAndReqId, reqID, resp.StatusCode),
		}
	}

	cr := new(GetIamUserResponse)
	err = decodeBody(resp, &cr)

	if err != nil {
		return nil, &AlksError{
			StatusCode: resp.StatusCode,
			RequestId:  reqID,
			Err:        fmt.Errorf("error parsing GetLongTermKeyResponse: [%s] %s", reqID, err),
		}
	}

	return cr, nil
}

func NewLongTermKeyRequest(options *CreateIamUserOptions) (*CreateIamUserRequest, error) {
	if options.IamUserName == nil {
		return nil, fmt.Errorf("IamUserName option must not be nil")
	}

	// iamUser := &CreateIamUserRequest{
	// 	User:{IamUserName: *options.IamUserName,},
	// }

	iamUser := &CreateIamUserRequest{}
	iamUser.User.IamUserName = *options.IamUserName

	if options.Tags != nil {
		iamUser.User.Tags = *options.Tags
	} else {
		iamUser.User.Tags = nil
	}

	return iamUser, nil
}

// CreateLongTermKey creates an LTK user for an account.
// If no error is returned, then you will receive an appropriate success message.
func (c *Client) CreateLongTermKey(options *CreateIamUserOptions) (*CreateIamUserResponse, error) {
	request, err := NewLongTermKeyRequest(options)
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        err,
		}
	}
	log.Printf("[INFO] Creating long term key: %s", *options.IamUserName)
	log.Printf("[INFO] The request body is %v", *request)

	accountID, err := c.AccountDetails.GetAccountNumber()
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        fmt.Errorf("Error reading Account value: %s", err),
		}
	}

	// request.AccountDetails = c.AccountDetails

	b, err := json.Marshal(struct {
		CreateIamUserRequest
	}{*request})
	log.Printf("[INFO] The byte array is %#v\n %s", string(b), string(b))

	// request := LongTermKeyRequest{
	// 	AccountDetails: c.AccountDetails,
	// 	IamUserName:    iamUsername,
	// }

	// reqBody, err := json.Marshal(request)

	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        fmt.Errorf("error encoding LTK create JSON: %s", err),
		}
	}

	req, err := c.NewRequest(b, "POST", "/iam-users/id/"+accountID)
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        err,
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &AlksError{
			StatusCode: 0,
			RequestId:  "",
			Err:        err,
		}
	}

	reqID := GetRequestID(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		keyErr := new(AlksResponseError)
		err = decodeBody(resp, &keyErr)

		if err != nil {
			return nil, &AlksError{
				StatusCode: resp.StatusCode,
				RequestId:  reqID,
				Err:        fmt.Errorf(ParseErrorReqId, reqID, err),
			}
		}

		if keyErr.Errors != nil {
			return nil, &AlksError{
				StatusCode: resp.StatusCode,
				RequestId:  reqID,
				Err:        fmt.Errorf(ErrorStringFull, reqID, resp.StatusCode, keyErr.Errors),
			}
		}

		return nil, &AlksError{
			StatusCode: resp.StatusCode,
			RequestId:  reqID,
			Err:        fmt.Errorf(ErrorStringOnlyCodeAndReqId, reqID, resp.StatusCode),
		}
	}

	cr := new(CreateIamUserResponse)
	err = decodeBody(resp, &cr)

	if err != nil {
		return nil, &AlksError{
			StatusCode: resp.StatusCode,
			RequestId:  reqID,
			Err:        fmt.Errorf("error parsing CreateLongTermKeyResponse: [%s] %s", reqID, err),
		}
	}
	return cr, nil
}

// DeleteLongTermKey deletes an LTK user for an account.
// If no error is returned, then you will receive an appropriate success message.
func (c *Client) DeleteLongTermKey(iamUsername string) (*DeleteIamUserResponse, error) {
	log.Printf("[INFO] Deleting long term key: %s", iamUsername)

	request := DeleteIamUserRequest{
		AccountDetails: c.AccountDetails,
		IamUserName:    iamUsername,
	}

	reqBody, err := json.Marshal(request)

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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		keyErr := new(AlksResponseError)
		err = decodeBody(resp, &keyErr)
		if err != nil {
			if reqID := GetRequestID(resp); reqID != "" {
				return nil, fmt.Errorf(ParseErrorReqId, reqID, err)
			}

			return nil, fmt.Errorf(ParseError, err)
		}

		if keyErr.Errors != nil {
			if reqID := GetRequestID(resp); reqID != "" {
				return nil, fmt.Errorf(ErrorStringFull, reqID, resp.StatusCode, keyErr.Errors)
			}

			return nil, fmt.Errorf(ErrorStringNoReqId, resp.StatusCode, keyErr.Errors)
		}

		if reqID := GetRequestID(resp); reqID != "" {
			return nil, fmt.Errorf(ErrorStringOnlyCodeAndReqId, reqID, resp.StatusCode)
		}

		return nil, fmt.Errorf(ErrorStringOnlyCode, resp.StatusCode)
	}

	cr := new(DeleteIamUserResponse)
	err = decodeBody(resp, &cr)

	if err != nil {
		if reqID := GetRequestID(resp); reqID != "" {
			return nil, fmt.Errorf("error parsing DeleteLongTermKeyResponse: [%s] %s", reqID, err)
		}

		return nil, fmt.Errorf("error parsing DeleteLongTermKeyResponse: %s", err)
	}

	// if cr.RequestFailed() {
	// 	return nil, fmt.Errorf("error deleting long term key: [%s] %s", cr.BaseResponse.RequestID, strings.Join(cr.GetErrors(), ", "))
	// }

	return cr, nil

}

func (c *Client) UpdateLongTermKey(options *UpdateIamUserRequest) (*UpdateIamUserResponse, error) {
	if err := options.updateLongTermKeyValidate(); err != nil {
		return nil, err
	}
	log.Printf("[INFO] update LTK %s with Tags: %v", *options.IamUserName, *options.Tags)

	b, err := json.Marshal(struct {
		UpdateIamUserRequest
		AccountDetails
	}{*options, c.AccountDetails})
	if err != nil {
		return nil, err
	}
	req, err := c.NewRequest(b, "PATCH", "/IAMUser/")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		updateErr := new(AlksResponseError)
		err = decodeBody(resp, &updateErr)

		if err != nil {
			if reqID := GetRequestID(resp); reqID != "" {
				return nil, fmt.Errorf(ParseErrorReqId, reqID, err)
			}

			return nil, fmt.Errorf(ParseError, err)
		}

		if updateErr.Errors != nil {
			if reqID := GetRequestID(resp); reqID != "" {
				return nil, fmt.Errorf(ErrorStringFull, reqID, resp.StatusCode, updateErr.Errors)
			}

			return nil, fmt.Errorf(ErrorStringNoReqId, resp.StatusCode, updateErr.Errors)
		}

		if reqID := GetRequestID(resp); reqID != "" {
			return nil, fmt.Errorf(ErrorStringOnlyCodeAndReqId, reqID, resp.StatusCode)
		}

		return nil, fmt.Errorf(ErrorStringOnlyCode, resp.StatusCode)
	}

	respObj := &UpdateIamUserResponse{}
	if err = decodeBody(resp, respObj); err != nil {
		if reqID := GetRequestID(resp); reqID != "" {
			return nil, fmt.Errorf("error parsing update ltk response: [%s] %s", reqID, err)
		}
		return nil, fmt.Errorf("error parsing update ltk response: %s", err)
	}
	// if respObj.RequestFailed() {
	// 	return nil, fmt.Errorf("error from update IAM ltk request: [%s] %s", respObj.RequestID, strings.Join(respObj.GetErrors(), ", "))
	// }

	return respObj, nil
}

func (req *UpdateIamUserRequest) updateLongTermKeyValidate() error {
	if req.IamUserName == nil {
		return fmt.Errorf("User name option must not be nil")
	}
	if req.Tags == nil {
		return fmt.Errorf("tags option must not be nil")
	}
	return nil
}
