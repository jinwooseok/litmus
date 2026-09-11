package fuzz_tests

import (
	"context"
	"errors"
	"testing"

	dbProbeMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/probe/model/mocks"

	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/chaos_experiment/handler"
	chaosExperimentMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/chaos_experiment/model/mocks"
	chaosExperimentRunMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/chaos_experiment_run/model/mocks"
	chaosInfraMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/chaos_infrastructure/model/mocks"
	dbMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/mocks"
	dbGitOpsMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/gitops/model/mocks"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"github.com/google/uuid"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/graph/model"
	store "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/data-store"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb"
	dbChaosExperiment "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/chaos_experiment"
	dbChaosExperimentRun "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/chaos_experiment_run"
	dbChoasInfra "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/chaos_infrastructure"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type MockServices struct {
	ChaosExperimentService     *chaosExperimentMocks.ChaosExperimentService
	ChaosExperimentRunService  *chaosExperimentRunMocks.ChaosExperimentRunService
	InfrastructureService      *chaosInfraMocks.InfraService
	GitOpsService              *dbGitOpsMocks.GitOpsService
	ChaosExperimentOperator    *dbChaosExperiment.Operator
	ChaosExperimentRunOperator *dbChaosExperimentRun.Operator
	MongodbOperator            *dbMocks.MongoOperator
	ChaosExperimentHandler     *handler.ChaosExperimentHandler
}

func NewMockServices() *MockServices {
	var (
		mongodbMockOperator        = new(dbMocks.MongoOperator)
		infrastructureService      = new(chaosInfraMocks.InfraService)
		chaosExperimentRunService  = new(chaosExperimentRunMocks.ChaosExperimentRunService)
		gitOpsService              = new(dbGitOpsMocks.GitOpsService)
		chaosExperimentOperator    = dbChaosExperiment.NewChaosExperimentOperator(mongodbMockOperator)
		chaosExperimentRunOperator = dbChaosExperimentRun.NewChaosExperimentRunOperator(mongodbMockOperator)
		chaosExperimentService     = new(chaosExperimentMocks.ChaosExperimentService)
		probeService               = new(dbProbeMocks.ProbeService)
	)
	var chaosExperimentHandler = handler.NewChaosExperimentHandler(chaosExperimentService, chaosExperimentRunService, infrastructureService, gitOpsService, chaosExperimentOperator, chaosExperimentRunOperator, probeService, mongodbMockOperator)
	return &MockServices{
		ChaosExperimentService:     chaosExperimentService,
		ChaosExperimentRunService:  chaosExperimentRunService,
		InfrastructureService:      infrastructureService,
		GitOpsService:              gitOpsService,
		ChaosExperimentOperator:    chaosExperimentOperator,
		ChaosExperimentRunOperator: chaosExperimentRunOperator,
		MongodbOperator:            mongodbMockOperator,
		ChaosExperimentHandler:     chaosExperimentHandler,
	}
}

var injectedErr = errors.New("injected mock failure")

func pickFailure(consumer *fuzz.ConsumeFuzzer, numCalls int) (failAt int, wantErr bool, err error) {
	raw, err := consumer.GetInt()
	if err != nil {
		return 0, false, err
	}
	span := numCalls + 1
	failAt = ((raw % span) + span) % span
	return failAt, failAt < numCalls, nil
}

func errIf(callIdx, failAt int) error {
	if callIdx == failAt {
		return injectedErr
	}
	return nil
}

func FuzzSaveChaosExperiment(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		experimentType := dbChaosExperiment.NonCronExperiment
		targetStruct := &struct {
			projectID string
			request   model.SaveChaosExperimentRequest
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		const (
			callGet = iota
			callProcessExperiment
			callUpsertGit
			callProcessUpdate
			numCalls
		)
		failAt, wantErr, err := pickFailure(fuzzConsumer, numCalls)
		if err != nil {
			return
		}

		ctx := context.Background()
		mockServices := NewMockServices()

		findResult := bson.D{
			{Key: "experiment_id", Value: targetStruct.request.ID},
			{Key: "name", Value: targetStruct.request.Name},
		}
		singleResult := mongo.NewSingleResultFromDocument(findResult, nil, nil)
		mockServices.MongodbOperator.On("Get", mock.Anything, mongodb.ChaosExperimentCollection, mock.Anything).
			Return(singleResult, errIf(callGet, failAt)).Once()

		experimentID := targetStruct.request.ID
		mockServices.ChaosExperimentService.On("ProcessExperiment", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&model.ChaosExperimentRequest{
			ExperimentID:   &experimentID,
			InfraID:        targetStruct.request.InfraID,
			ExperimentType: &model.AllExperimentType[0],
		}, &experimentType, errIf(callProcessExperiment, failAt)).Once()

		mockServices.GitOpsService.On("UpsertExperimentToGit", ctx, mock.Anything, mock.Anything).
			Return(errIf(callUpsertGit, failAt)).Once()

		mockServices.ChaosExperimentService.On("ProcessExperimentUpdate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, mock.Anything, mock.Anything).
			Return(errIf(callProcessUpdate, failAt)).Once()

		res, err := mockServices.ChaosExperimentHandler.SaveChaosExperiment(ctx, targetStruct.request, targetStruct.projectID, "")
		if (err != nil) != wantErr {
			t.Errorf("ChaosExperimentHandler.SaveChaosExperiment() error = %v, wantErr %v", err, wantErr)
			return
		}
		if !wantErr && res == "" {
			t.Errorf("Returned environment is nil")
		}
	})
}

func FuzzDeleteChaosExperiment(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			projectID       string
			experimentId    string
			experimentRunID string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		const (
			callGetExperiment = iota
			callGetExperimentRun
			callDeleteFromGit
			callProcessRunDelete
			numCalls
		)
		failAt, wantErr, err := pickFailure(fuzzConsumer, numCalls)
		if err != nil {
			return
		}

		runID := "run-" + targetStruct.experimentRunID

		ctx := context.Background()
		findResult := bson.D{
			{Key: "experiment_id", Value: targetStruct.experimentId},
		}
		singleResult := mongo.NewSingleResultFromDocument(findResult, nil, nil)

		mockServices := NewMockServices()
		mockServices.MongodbOperator.On("Get", mock.Anything, mongodb.ChaosExperimentCollection, mock.Anything).
			Return(singleResult, errIf(callGetExperiment, failAt)).Once()
		mockServices.MongodbOperator.On("Get", mock.Anything, mongodb.ChaosExperimentRunsCollection, mock.Anything).
			Return(singleResult, errIf(callGetExperimentRun, failAt)).Once()
		mockServices.GitOpsService.On("DeleteExperimentFromGit", mock.Anything, mock.Anything, mock.Anything).
			Return(errIf(callDeleteFromGit, failAt)).Once()
		mockServices.ChaosExperimentRunService.On("ProcessExperimentRunDelete", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errIf(callProcessRunDelete, failAt)).Once()

		store := store.NewStore()
		res, err := mockServices.ChaosExperimentHandler.DeleteChaosExperiment(ctx, targetStruct.projectID, targetStruct.experimentId, &runID, store, "")
		if (err != nil) != wantErr {
			t.Errorf("ChaosExperimentHandler.DeleteChaosExperiment() error = %v, wantErr %v", err, wantErr)
			return
		}
		if !wantErr && res == false {
			t.Errorf("Returned response is false")
		}
	})
}

func FuzzUpdateChaosExperiment(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			projectID  string
			experiment model.ChaosExperimentRequest
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		const (
			callList = iota
			callProcessExperiment
			callUpsertGit
			callProcessUpdate
			numCalls
		)
		failAt, wantErr, err := pickFailure(fuzzConsumer, numCalls)
		if err != nil {
			return
		}

		experimentType := dbChaosExperiment.NonCronExperiment
		ctx := context.Background()
		mockServices := NewMockServices()

		cursor, _ := mongo.NewCursorFromDocuments(nil, nil, nil)
		mockServices.MongodbOperator.On("List", mock.Anything, mongodb.ChaosExperimentCollection, mock.Anything).
			Return(cursor, errIf(callList, failAt)).Once()

		experimentID := uuid.New().String()
		mockServices.ChaosExperimentService.On("ProcessExperiment", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&model.ChaosExperimentRequest{
			ExperimentID:   &experimentID,
			InfraID:        "abc",
			ExperimentType: &model.AllExperimentType[0],
		}, &experimentType, errIf(callProcessExperiment, failAt)).Once()

		mockServices.GitOpsService.On("UpsertExperimentToGit", ctx, mock.Anything, mock.Anything).
			Return(errIf(callUpsertGit, failAt)).Once()

		mockServices.ChaosExperimentService.On("ProcessExperimentUpdate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errIf(callProcessUpdate, failAt)).Once()

		store := store.NewStore()
		res, err := mockServices.ChaosExperimentHandler.UpdateChaosExperiment(ctx, targetStruct.experiment, targetStruct.projectID, store, "")
		if (err != nil) != wantErr {
			t.Errorf("ChaosExperimentHandler.UpdateChaosExperiment() error = %v, wantErr %v", err, wantErr)
			return
		}
		if !wantErr && res == nil {
			t.Errorf("Returned response is nil")
		}
	})
}

func FuzzGetExperiment(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			projectID    string
			experimentId string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		const numCalls = 1
		failAt, wantErr, err := pickFailure(fuzzConsumer, numCalls)
		if err != nil {
			return
		}

		ctx := context.Background()
		mockServices := NewMockServices()

		var cursor *mongo.Cursor
		if !wantErr {
			findResult := []interface{}{bson.D{
				{Key: "project_id", Value: targetStruct.projectID},
				{Key: "infra_id", Value: "abc"},
				{Key: "kubernetesInfraDetails", Value: []dbChoasInfra.ChaosInfra{
					{
						ProjectID: targetStruct.projectID,
						InfraID:   "abc",
					},
				}},
				{
					Key: "revision", Value: []dbChaosExperiment.ExperimentRevision{
						{
							RevisionID: uuid.NewString(),
						},
					},
				},
			}}
			cursor, _ = mongo.NewCursorFromDocuments(findResult, nil, nil)
		}
		mockServices.MongodbOperator.On("Aggregate", mock.Anything, mongodb.ChaosExperimentCollection, mock.Anything, mock.Anything).
			Return(cursor, errIf(0, failAt)).Once()

		res, err := mockServices.ChaosExperimentHandler.GetExperiment(ctx, targetStruct.experimentId, targetStruct.projectID)
		if (err != nil) != wantErr {
			t.Errorf("ChaosExperimentHandler.GetExperiment() error = %v, wantErr %v", err, wantErr)
			return
		}
		if !wantErr && res == nil {
			t.Errorf("Returned response is nil")
		}
	})
}

func FuzzListExperiment(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			projectID string
			request   model.ListExperimentRequest
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		if targetStruct.request.Filter != nil {
			targetStruct.request.Filter.DateRange = nil
		}

		const numCalls = 1
		failAt, wantErr, err := pickFailure(fuzzConsumer, numCalls)
		if err != nil {
			return
		}

		mockServices := NewMockServices()

		var cursor *mongo.Cursor
		if !wantErr {
			findResult := []interface{}{
				bson.D{
					{Key: "project_id", Value: targetStruct.projectID},
					{Key: "infra_id", Value: "abc"},
				}}
			cursor, _ = mongo.NewCursorFromDocuments(findResult, nil, nil)
		}
		mockServices.MongodbOperator.On("Aggregate", mock.Anything, mongodb.ChaosExperimentCollection, mock.Anything, mock.Anything).
			Return(cursor, errIf(0, failAt)).Once()

		res, err := mockServices.ChaosExperimentHandler.ListExperiment(targetStruct.projectID, targetStruct.request)
		if (err != nil) != wantErr {
			t.Errorf("ChaosExperimentHandler.ListExperiment() error = %v, wantErr %v", err, wantErr)
			return
		}
		if !wantErr && res == nil {
			t.Errorf("Returned response is nil")
		}
	})
}

func FuzzDisableCronExperiment(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			projectID string
			username  string
			request   dbChaosExperiment.ChaosExperimentRequest
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		if len(targetStruct.request.Revision) < 1 {
			return
		}
		targetStruct.request.Revision[len(targetStruct.request.Revision)-1].ExperimentManifest = "{}"

		const numCalls = 1
		failAt, wantErr, err := pickFailure(fuzzConsumer, numCalls)
		if err != nil {
			return
		}

		mockServices := NewMockServices()
		mockServices.ChaosExperimentService.On("ProcessExperimentUpdate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errIf(0, failAt)).Once()

		store := store.NewStore()
		err = mockServices.ChaosExperimentHandler.DisableCronExperiment(targetStruct.username, targetStruct.request, targetStruct.projectID, store)
		if (err != nil) != wantErr {
			t.Errorf("ChaosExperimentHandler.DisableCronExperiment() error = %v, wantErr %v", err, wantErr)
		}
	})
}

func FuzzGetExperimentStats(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			projectID string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		const numCalls = 1
		failAt, wantErr, err := pickFailure(fuzzConsumer, numCalls)
		if err != nil {
			return
		}

		ctx := context.Background()
		mockServices := NewMockServices()

		var cursor *mongo.Cursor
		if !wantErr {
			findResult := []interface{}{
				bson.D{
					{Key: "project_id", Value: targetStruct.projectID},
				},
			}
			cursor, _ = mongo.NewCursorFromDocuments(findResult, nil, nil)
		}
		mockServices.MongodbOperator.On("Aggregate", mock.Anything, mongodb.ChaosExperimentCollection, mock.Anything, mock.Anything).
			Return(cursor, errIf(0, failAt)).Once()

		res, err := mockServices.ChaosExperimentHandler.GetExperimentStats(ctx, targetStruct.projectID)
		if (err != nil) != wantErr {
			t.Errorf("ChaosExperimentHandler.GetExperimentStats() error = %v, wantErr %v", err, wantErr)
			return
		}

		if !wantErr && res == nil {
			t.Errorf("response is nil")
		}

	})
}
