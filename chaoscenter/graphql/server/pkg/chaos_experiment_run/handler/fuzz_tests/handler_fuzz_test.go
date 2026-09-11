package fuzz_tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	dbProbeMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/probe/model/mocks"

	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/chaos_experiment_run/handler"
	chaosInfraMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/chaos_infrastructure/model/mocks"
	dbChaosExperiment "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/chaos_experiment"
	dbChaosExperimentRun "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/chaos_experiment_run"
	dbMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/mocks"
	dbGitOpsMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/gitops/model/mocks"

	fuzz "github.com/AdaLogics/go-fuzz-headers"

	"github.com/google/uuid"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/graph/model"
	typesMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/chaos_experiment_run/model/mocks"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type MockServices struct {
	ChaosExperimentRunService  *typesMocks.ChaosExperimentRunService
	InfrastructureService      *chaosInfraMocks.InfraService
	GitOpsService              *dbGitOpsMocks.GitOpsService
	ChaosExperimentOperator    *dbChaosExperiment.Operator
	ChaosExperimentRunOperator *dbChaosExperimentRun.Operator
	MongodbOperator            *dbMocks.MongoOperator
	ChaosExperimentRunHandler  *handler.ChaosExperimentRunHandler
	ProbeService               *dbProbeMocks.ProbeService
}

func NewMockServices() *MockServices {
	var (
		mongodbMockOperator        = new(dbMocks.MongoOperator)
		infrastructureService      = new(chaosInfraMocks.InfraService)
		gitOpsService              = new(dbGitOpsMocks.GitOpsService)
		chaosExperimentRunService  = new(typesMocks.ChaosExperimentRunService)
		chaosExperimentOperator    = dbChaosExperiment.NewChaosExperimentOperator(mongodbMockOperator)
		chaosExperimentRunOperator = dbChaosExperimentRun.NewChaosExperimentRunOperator(mongodbMockOperator)
		probeService               = new(dbProbeMocks.ProbeService)
	)
	var chaosExperimentRunHandler = handler.NewChaosExperimentRunHandler(
		chaosExperimentRunService,
		infrastructureService,
		gitOpsService,
		chaosExperimentOperator,
		chaosExperimentRunOperator,
		probeService,
		mongodbMockOperator,
	)
	return &MockServices{
		ChaosExperimentRunService:  chaosExperimentRunService,
		InfrastructureService:      infrastructureService,
		GitOpsService:              gitOpsService,
		ChaosExperimentOperator:    chaosExperimentOperator,
		ChaosExperimentRunOperator: chaosExperimentRunOperator,
		MongodbOperator:            mongodbMockOperator,
		ProbeService:               probeService,
		ChaosExperimentRunHandler:  chaosExperimentRunHandler,
	}
}

func FuzzGetExperimentRun(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			ProjectID       string
			ExperimentRunID string
			NotifyID        string
		}{}

		targetStruct.ProjectID = uuid.New().String()

		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		shouldErr, err := fuzzConsumer.GetBool()
		if err != nil {
			return
		}

		ctx := context.Background()
		mockServices := NewMockServices()
		findResult := []interface{}{bson.D{
			{Key: "experiment_run_id", Value: targetStruct.ExperimentRunID},
			{Key: "project_id", Value: targetStruct.ProjectID},
			{Key: "infra_id", Value: "mockInfraID"},
			{Key: "kubernetesInfraDetails", Value: bson.A{
				bson.D{
					{Key: "InfraID", Value: "mockInfraID"},
					{Key: "Name", Value: "MockInfra"},
					{Key: "EnvironmentID", Value: "mockEnvID"},
					{Key: "Description", Value: "Mock Infrastructure"},
					{Key: "PlatformName", Value: "Kubernetes"},
					{Key: "IsActive", Value: true},
					{Key: "UpdatedAt", Value: time.Now().Unix()},
					{Key: "CreatedAt", Value: time.Now().Unix()},
				},
			}},
			{Key: "experiment", Value: bson.A{
				bson.D{
					{Key: "ExperimentName", Value: "MockExperiment"},
					{Key: "ExperimentType", Value: "MockType"},
					{Key: "Revision", Value: bson.A{
						bson.D{
							{Key: "RevisionID", Value: uuid.NewString()},
							{Key: "ExperimentManifest", Value: "mockManifest"},
							{Key: "Weightages", Value: bson.A{
								bson.D{{Key: "FaultName", Value: "fault1"}, {Key: "Weightage", Value: 10}},
								bson.D{{Key: "FaultName", Value: "fault2"}, {Key: "Weightage", Value: 20}},
							}},
						},
					}},
				},
			}},
		}}

		var (
			cursor *mongo.Cursor
			aggErr error
		)
		if shouldErr {
			aggErr = errors.New("mocked aggregate failure")
		} else {
			cursor, _ = mongo.NewCursorFromDocuments(findResult, nil, nil)
		}
		mockServices.MongodbOperator.On("Aggregate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(cursor, aggErr).Once()

		res, err := mockServices.ChaosExperimentRunHandler.GetExperimentRun(ctx, targetStruct.ProjectID, &targetStruct.ExperimentRunID, &targetStruct.NotifyID)
		if (err != nil) != shouldErr {
			t.Errorf("ChaosExperimentRunHandler.GetExperimentRun() error = %v, shouldErr %v", err, shouldErr)
			return
		}
		if !shouldErr && res == nil {
			t.Errorf("Returned response is nil")
		}
	})
}

func FuzzListExperimentRun(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			ProjectID string
			Request   model.ListExperimentRunRequest
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		shouldErr, err := fuzzConsumer.GetBool()
		if err != nil {
			return
		}
		targetStruct.Request.Filter = nil

		mockServices := NewMockServices()
		findResult := []interface{}{bson.D{
			{Key: "project_id", Value: targetStruct.ProjectID},
			{Key: "infra_id", Value: "abc"},
			{
				Key: "revision", Value: []dbChaosExperiment.ExperimentRevision{
					{
						RevisionID: uuid.NewString(),
					},
				},
			},
		}}
		var (
			cursor *mongo.Cursor
			aggErr error
		)
		if shouldErr {
			aggErr = errors.New("mocked aggregate failure")
		} else {
			cursor, _ = mongo.NewCursorFromDocuments(findResult, nil, nil)
		}
		mockServices.MongodbOperator.On("Aggregate", mock.Anything, mongodb.ChaosExperimentRunsCollection, mock.Anything, mock.Anything).Return(cursor, aggErr).Once()

		res, err := mockServices.ChaosExperimentRunHandler.ListExperimentRun(targetStruct.ProjectID, targetStruct.Request)
		if (err != nil) != shouldErr {
			t.Errorf("ListExperimentRun() error = %v, shouldErr %v", err, shouldErr)
			return
		}
		if !shouldErr && res == nil {
			t.Errorf("Returned response is nil")
		}

	})
}

func FuzzRunChaosWorkFlow(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			ProjectID string
			Workflow  dbChaosExperiment.ChaosExperimentRequest
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		shouldErr, err := fuzzConsumer.GetBool()
		if err != nil {
			return
		}

		mockServices := NewMockServices()

		findResult := []interface{}{bson.D{
			{Key: "infra_id", Value: targetStruct.ProjectID},
		}}
		var getErr error
		if shouldErr {
			getErr = errors.New("mocked infra fetch failure")
		}
		singleResult := mongo.NewSingleResultFromDocument(findResult[0], getErr, nil)
		mockServices.MongodbOperator.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(singleResult, nil).Once()

		_, err = mockServices.ChaosExperimentRunHandler.RunChaosWorkFlow(context.Background(), targetStruct.ProjectID, targetStruct.Workflow, nil)
		if err == nil {
			t.Errorf("RunChaosWorkFlow() expected an error (either infra fetch failure or inactive infra), got nil")
			return
		}
		if shouldErr {
			return
		}
		if !strings.Contains(err.Error(), "inactive infra") {
			t.Errorf("RunChaosWorkFlow() error = %v, want inactive infra error", err)
		}
	})
}

func FuzzGetExperimentRunStats(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			ProjectID string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		targetStruct.ProjectID = uuid.New().String()

		shouldErr, err := fuzzConsumer.GetBool()
		if err != nil {
			return
		}

		mockServices := NewMockServices()

		findResult := []interface{}{bson.D{
			{Key: "project_id", Value: targetStruct.ProjectID},
			{Key: "infra_id", Value: "abc"},
			{
				Key: "revision", Value: []dbChaosExperiment.ExperimentRevision{
					{
						RevisionID: uuid.NewString(),
					},
				},
			},
		}}
		var (
			cursor *mongo.Cursor
			aggErr error
		)
		if shouldErr {
			aggErr = errors.New("mocked aggregate failure")
		} else {
			cursor, _ = mongo.NewCursorFromDocuments(findResult, nil, nil)
		}
		mockServices.MongodbOperator.On("Aggregate", mock.Anything, mongodb.ChaosExperimentRunsCollection, mock.Anything, mock.Anything).Return(cursor, aggErr).Once()

		res, err := mockServices.ChaosExperimentRunHandler.GetExperimentRunStats(context.Background(), targetStruct.ProjectID)
		if (err != nil) != shouldErr {
			t.Errorf("GetExperimentRunStats() error = %v, shouldErr %v", err, shouldErr)
			return
		}
		if !shouldErr && res == nil {
			t.Errorf("Returned response is nil")
		}
	})
}
