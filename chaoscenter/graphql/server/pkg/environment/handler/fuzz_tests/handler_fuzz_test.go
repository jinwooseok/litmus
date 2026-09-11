package fuzz_tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	dbOperationsEnvironment "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/environments"
	dbMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/mocks"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/environment/handler"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/graph/model"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/authorization"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	mongodbMockOperator = new(dbMocks.MongoOperator)
	environmentOperator = dbOperationsEnvironment.NewEnvironmentOperator(mongodbMockOperator)
)

var JwtSecret = "testsecret"

func GetSignedJWT(name string) (string, error) {
	token := jwt.New(jwt.SigningMethodHS512)
	claims := token.Claims.(jwt.MapClaims)
	claims["uid"] = uuid.NewString()
	claims["role"] = uuid.NewString()
	claims["username"] = name
	claims["exp"] = time.Now().Add(time.Minute).Unix()

	tokenString, err := token.SignedString([]byte(JwtSecret))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

func FuzzCreateEnvironment(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			input     model.CreateEnvironmentRequest
			projectID string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		shouldErr, err := fuzzConsumer.GetBool()
		if err != nil {
			return
		}
		var createErr error
		if shouldErr {
			createErr = errors.New("mocked mongo create error")
		}
		mongodbMockOperator.On("Create", mock.Anything, mongodb.EnvironmentCollection, mock.Anything).Return(createErr).Once()

		token, err := GetSignedJWT("testUser")
		if err != nil {
			logrus.Errorf("Error genrating token %v", err)
		}

		ctx := context.WithValue(context.Background(), authorization.AuthKey, token)
		service := handler.NewEnvironmentService(environmentOperator)

		env, err := service.CreateEnvironment(ctx, targetStruct.projectID, &targetStruct.input, "")
		if (err != nil) != shouldErr {
			t.Errorf("CreateEnvironment() error = %v, shouldErr %v", err, shouldErr)
			return
		}
		if !shouldErr && env == nil {
			t.Errorf("Returned environment is nil")
		}
	})
}

func FuzzTestDeleteEnvironment(f *testing.F) {
	testCases := []struct {
		projectID     string
		environmentID string
		failAt        int
	}{
		{
			projectID:     "testProject",
			environmentID: "testEnvID",
			failAt:        0,
		},
		{
			projectID:     "testProject",
			environmentID: "testEnvID",
			failAt:        1,
		},
		{
			projectID:     "testProject",
			environmentID: "testEnvID",
			failAt:        2,
		},
	}
	for _, tc := range testCases {
		f.Add(tc.projectID, tc.environmentID, tc.failAt)
	}

	f.Fuzz(func(t *testing.T, projectID string, environmentID string, failAt int) {
		mode := failAt % 3
		if mode < 0 {
			mode += 3
		}

		findResult := []interface{}{bson.D{
			{Key: "environment_id", Value: environmentID},
			{Key: "project_id", Value: projectID},
		}}

		var (
			getErr        error
			updateManyErr error
		)
		switch mode {
		case 0:
			getErr = errors.New("mocked mongo get error")
		case 1:
			updateManyErr = errors.New("mocked mongo update error")
		}

		getResult := mongo.NewSingleResultFromDocument(findResult[0], getErr, nil)
		mongodbMockOperator.On("Get", mock.Anything, mongodb.EnvironmentCollection, mock.Anything).Return(getResult, nil).Once()

		if mode != 0 {
			mongodbMockOperator.On("UpdateMany", mock.Anything, mongodb.EnvironmentCollection, mock.Anything, mock.Anything, mock.Anything).Return(&mongo.UpdateResult{}, updateManyErr).Once()
		}

		token, err := GetSignedJWT("testUser")
		if err != nil {
			logrus.Errorf("Error genrating token %v", err)
		}

		ctx := context.WithValue(context.Background(), authorization.AuthKey, token)
		service := handler.NewEnvironmentService(environmentOperator)

		wantErr := mode != 2
		env, err := service.DeleteEnvironment(ctx, projectID, environmentID, "")
		if (err != nil) != wantErr {
			t.Errorf("DeleteEnvironment() error = %v, wantErr %v (failAt mode %d)", err, wantErr, mode)
			return
		}
		if !wantErr && env == "" {
			t.Errorf("Returned environment is empty")
		}
	})
}

func FuzzTestGetEnvironment(f *testing.F) {
	testCases := []struct {
		projectID     string
		environmentID string
		shouldErr     bool
	}{
		{
			projectID:     "testProject",
			environmentID: "testEnvID",
			shouldErr:     false,
		},
		{
			projectID:     "testProject",
			environmentID: "testEnvID",
			shouldErr:     true,
		},
	}
	for _, tc := range testCases {
		f.Add(tc.projectID, tc.environmentID, tc.shouldErr)
	}

	f.Fuzz(func(t *testing.T, projectID string, environmentID string, shouldErr bool) {

		findResult := []interface{}{bson.D{
			{Key: "environment_id", Value: environmentID},
			{Key: "project_id", Value: projectID},
		}}

		var getErr error
		if shouldErr {
			getErr = errors.New("mocked mongo get error")
		}

		singleResult := mongo.NewSingleResultFromDocument(findResult[0], getErr, nil)
		mongodbMockOperator.On("Get", mock.Anything, mongodb.EnvironmentCollection, mock.Anything).Return(singleResult, nil).Once()
		service := handler.NewEnvironmentService(environmentOperator)

		env, err := service.GetEnvironment(projectID, environmentID)
		if (err != nil) != shouldErr {
			t.Errorf("GetEnvironment() error = %v, shouldErr %v", err, shouldErr)
			return
		}
		if !shouldErr && env == nil {
			t.Errorf("Returned environment is nil")
		}
	})
}
