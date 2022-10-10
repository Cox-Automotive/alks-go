package alks

import (
	. "github.com/motain/gocheck"
)

func (s *S) Test_GetLongTermKeys(c *C) {
	testServer.Response(202, nil, longTermKeys)

	resp, err := s.client.GetLongTermKeys()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.LongTermKeys, DeepEquals, []LongTermKey{{UserName: "bob", AccessKeyID: "verySecret", Status: "Active", CreateDate: "2020-04-20"}})
}

func (s *S) Test_GetLongTermKeyNoTags(c *C) {
	testServer.Response(202, nil, longTermKey)

	resp, err := s.client.GetLongTermKey("bob")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.LongTermKey, DeepEquals, LongTermKey{})

}

func (s *S) Test_GetLongTermKeyWithTags(c *C) {
	testServer.Response(202, nil, longTermKeyWithTags)

	resp, err := s.client.GetLongTermKey("bob")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.LongTermKey, DeepEquals, LongTermKey{})
	c.Assert(resp.Tags[0].Key, Equals, "foo")
	c.Assert(resp.Tags[0].Value, Equals, "bar")
	c.Assert(resp.Tags[1].Key, Equals, "cloud")
	c.Assert(resp.Tags[1].Value, Equals, "railway")
}

func (s *S) Test_CreateLongTermKeysNoIamUserName(c *C) {
	testServer.Response(202, nil, createLongTermKeysNoTags)
	tags := []Tag{
		{
			Key:   "foo",
			Value: "bar",
		},
		{
			Key:   "cloud",
			Value: "railway",
		},
	}

	options := CreateLongTermKeyOptions{
		Tags: &tags,
	}

	resp, err := s.client.CreateLongTermKey(&options)

	c.Assert(err, NotNil)
	c.Assert(resp, IsNil)
}

func (s *S) Test_CreateLongTermKeysNoTags(c *C) {
	testServer.Response(202, nil, createLongTermKeysNoTags)

	username := "MY_USERNAME" 

	options := CreateLongTermKeyOptions{
		IamUserName: &username,
	}

	resp, err := s.client.CreateLongTermKey(&options)

	c.Assert(err, IsNil)
	c.Assert(resp.CreateLongTermKey, DeepEquals, CreateLongTermKey{
		IAMUserName: "MY_USERNAME",
		IAMUserArn:  "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
		AccessKey:   "thisismykey",
		SecretKey:   "secret/thisismysecret",
	})
	c.Assert(resp.BaseLongTermKeyResponse, DeepEquals, BaseLongTermKeyResponse{
		Action:              "CREATE",
		AddedIAMUserToGroup: true,
		PartialError:        false,
	})
}

func (s *S) Test_CreateLongTermKeysWithTags(c *C) {
	testServer.Response(202, nil, createLongTermKeysWithTags)

	username := "MY_USERNAME" 

	tags := []Tag{
		{
			Key:   "foo",
			Value: "bar",
		},
		{
			Key:   "cloud",
			Value: "railway",
		},
	}

	options := CreateLongTermKeyOptions{
		IamUserName: &username,
		Tags: &tags,
	}

	resp, err := s.client.CreateLongTermKey(&options)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.CreateLongTermKey, DeepEquals, CreateLongTermKey{
		IAMUserName: "MY_USERNAME",
		IAMUserArn:  "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
		AccessKey:   "thisismykey",
		SecretKey:   "secret/thisismysecret",
	})
	c.Assert(resp.BaseLongTermKeyResponse, DeepEquals, BaseLongTermKeyResponse{
		Action:              "CREATE",
		AddedIAMUserToGroup: true,
		PartialError:        false,
	})
}

func (s *S) Test_DeleteLongTermKeys(c *C) {
	testServer.Response(202, nil, deleteLongTermKeys)

	resp, err := s.client.DeleteLongTermKey("MyUserName")

	c.Assert(err, IsNil)
	c.Assert(resp.BaseLongTermKeyResponse, DeepEquals, BaseLongTermKeyResponse{
		AddedIAMUserToGroup: false,
		PartialError:        false,
	})
}

var deleteLongTermKeys = `
{
  "addedIAMUserToGroup": false,
  "partialError": false
}
`

var createLongTermKeysNoTags = `
{
    "account": "012345678910/ALKSAdmin - AccountAlias",
    "action": "CREATE",
    "iamUserName": "MY_USERNAME",
    "iamUserArn": "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
    "addedIAMUserToGroup": true,
    "partialError": false,
	"accessKey": "thisismykey",
    "secretKey": "secret/thisismysecret"
}
`

var createLongTermKeysWithTags = `
{
    "account": "012345678910/ALKSAdmin - AccountAlias",
    "action": "CREATE",
    "iamUserName": "MY_USERNAME",
    "iamUserArn": "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
    "addedIAMUserToGroup": true,
    "partialError": false,
	"accessKey": "thisismykey",
    "secretKey": "secret/thisismysecret",
	"tags": [
        {
            "key": "foo",
            "value": "bar"
        },
        {
            "key": "cloud",
            "value": "railway"
        }
    ]
}
`

var longTermKeys = `
{
	"longTermKeys": [
		{
			"userName": "bob",
			"accessKeyId": "verySecret",
			"status": "Active",
			"createDate": "2020-04-20"
		}
	]
}
`

var longTermKey = `
{
	"userName": "bob",
	"accessKeyId": "verySecret",
	"status": "Active",
	"createDate": "2020-04-20"
}
`

var longTermKeyWithTags = `
{
	"userName": "bob",
	"accessKeyId": "verySecret",
	"status": "Active",
	"createDate": "2020-04-20",
	"tags": [
        {
            "key": "foo",
            "value": "bar"
        },
        {
            "key": "cloud",
            "value": "railway"
        }
    ]
}
`
