package azure

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/google/uuid"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	log "github.com/sirupsen/logrus"
)

const (
	AzureStorageConnectionString = "AZURE_STORAGE_CONNECTION_STRING"
	CnabStateStorageAccountKey   = "CNAB_AZURE_STATE_STORAGE_ACCOUNT_KEY"
	timeout                      = 60
)

var StorageAccountName string
var StorageAccountKey string
var StateTableName string
var AsyncOperationTableName string

type RPState struct {
	*storage.Entity
}

type AsyncOperationState struct {
	Action string
	Status string
	Output string
}

func getTableServiceClient() (*storage.TableServiceClient, error) {
	client, err := storage.NewBasicClient(StorageAccountName, StorageAccountKey)
	if err != nil {
		return nil, fmt.Errorf("Failed to get Table Service Client: %v", err)
	}
	tableService := client.GetTableService()
	return &tableService, nil
}

// TODO get guid from header/context

func GetRPState(partitionKey string, resourceId string) (*models.BundleCommandProperties, error) {
	client, err := getTableServiceClient()
	if err != nil {
		return nil, err
	}
	rowkey := getRowKeyFromResourceId(resourceId)
	table := client.GetTableReference(StateTableName)
	row := table.GetEntityReference(partitionKey, rowkey)
	guid := uuid.New().String()
	log.Debugf("Get RP State for parition key: %s row key: %s id: %s", partitionKey, rowkey, guid)
	options := storage.GetEntityOptions{
		RequestID: guid,
	}
	err = row.Get(timeout, storage.MinimalMetadata, &options)
	if err != nil {
		log.Debugf("Failed to GET state for %s", resourceId)
		return nil, err
	}
	properties := models.BundleCommandProperties{}

	if params, ok := row.Properties["Parameters"].(string); ok {
		err = json.Unmarshal([]byte(params), &properties.Parameters)
		if err != nil {
			return nil, fmt.Errorf("Failed to de-serialise parameters: %v", err)
		}
	}

	if creds, ok := row.Properties["Credentials"].(string); ok {
		err = json.Unmarshal([]byte(creds), &properties.Credentials)
		if err != nil {
			return nil, fmt.Errorf("Failed to de-serialise credentials: %v", err)
		}
	}

	if errorResponse, ok := row.Properties["ErrorResponse"].([]byte); ok {
		// decompress error resp to avoid table storage size limit
		byteReader := bytes.NewReader(errorResponse)
		reader, err := gzip.NewReader(byteReader)
		if err != nil {
			return nil, err
		}
		var result []byte
		if _, err = reader.Read(result); err != nil {
			return nil, err
		}
		err = json.Unmarshal(result, &properties.ErrorResponse)
		if err != nil {
			return nil, fmt.Errorf("Failed to de-serialise error response: %v", err)
		}
	}

	properties.ProvisioningState = row.Properties["ProvisioningState"].(string)
	if val, ok := row.Properties["OperationId"].(string); ok {
		properties.OperationId = val
	}
	return &properties, nil
}

func PutRPState(partitionKey string, resourceId string, properties *models.BundleCommandProperties) error {
	client, err := getTableServiceClient()
	if err != nil {
		return err
	}
	rowkey := getRowKeyFromResourceId(resourceId)
	table := client.GetTableReference(StateTableName)
	row := table.GetEntityReference(partitionKey, rowkey)
	p := make(map[string]interface{})
	params, err := json.Marshal(properties.Parameters)
	if err != nil {
		return fmt.Errorf("Failed to serialise parameters:%v", err)
	}
	creds, err := json.Marshal(properties.Credentials)
	if err != nil {
		return fmt.Errorf("Failed to serialise creds:%v", err)
	}
	// TODO use reflection
	p["Parameters"] = string(params)
	p["Credentials"] = string(creds)
	p["ProvisioningState"] = properties.ProvisioningState
	p["OperationId"] = properties.OperationId
	p["ErrorResponse"] = nil
	p["ResourceProvider"] = properties.BundleInformation.ResourceProvider
	p["ResourceType"] = properties.BundleInformation.ResourceType
	p["Status"] = properties.Status
	row.Properties = p
	guid := uuid.New().String()
	options := storage.EntityOptions{
		Timeout:   timeout,
		RequestID: guid,
	}
	log.Debugf("Put RP State for parition key: %s row key: %s id: %s", partitionKey, rowkey, guid)
	return row.InsertOrReplace(&options)
}

func DeleteRPState(partitionKey string, resourceId string) error {
	client, err := getTableServiceClient()
	if err != nil {
		return err
	}
	table := client.GetTableReference(StateTableName)
	rowkey := getRowKeyFromResourceId(resourceId)
	row := table.GetEntityReference(partitionKey, rowkey)
	guid := uuid.New().String()
	options := storage.EntityOptions{
		Timeout:   timeout,
		RequestID: guid,
	}
	log.Debugf("Delete RP State for parition key: %s row key: %s id: %s", partitionKey, rowkey, guid)
	return row.Delete(true, &options)
}

func SetFailedProvisioningState(partitionKey string, resourceId string, errorResponse *helpers.ErrorResponse) error {
	client, err := getTableServiceClient()
	if err != nil {
		return err
	}
	rowkey := getRowKeyFromResourceId(resourceId)
	table := client.GetTableReference(StateTableName)
	row := table.GetEntityReference(partitionKey, rowkey)
	p := make(map[string]interface{})
	errResp, err := json.Marshal(errorResponse)
	if err != nil {
		return fmt.Errorf("Failed to serialise ErrorResponse:%v", err)
	}
	p["ProvisioningState"] = helpers.ProvisioningStateFailed
	// compress error resp to avoid table storage size limit
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := zw.Write(errResp); err != nil {
		return err
	}
	if err = zw.Close(); err != nil {
		return err
	}
	p["ErrorResponse"] = buf.Bytes()
	row.Properties = p
	guid := uuid.New().String()
	options := storage.EntityOptions{
		Timeout:   timeout,
		RequestID: guid,
	}
	log.Debugf("SetFailedProvisioningState for parition key: %s row key: %s id: %s", partitionKey, rowkey, guid)
	if err = row.Merge(true, &options); err != nil {
		return fmt.Errorf("Failed to SetFailedProvisioningState ErrorResponse:%v", err)
	}
	return nil
}

func UpdateRPStatus(partitionKey string, resourceId string, status string) error {
	client, err := getTableServiceClient()
	if err != nil {
		return err
	}
	rowkey := getRowKeyFromResourceId(resourceId)
	table := client.GetTableReference(StateTableName)
	row := table.GetEntityReference(partitionKey, rowkey)
	p := make(map[string]interface{})
	p["Status"] = status
	row.Properties = p
	guid := uuid.New().String()
	options := storage.EntityOptions{
		Timeout:   timeout,
		RequestID: guid,
	}
	log.Debugf("Update RP status for parition key: %s row key: %s id: %s", partitionKey, rowkey, guid)
	if err = row.Merge(true, &options); err != nil {
		return fmt.Errorf("Failed to update RP status:%v", err)
	}
	return nil
}

func ListRPState(partitionKey string, resourceProviderName string, resourceTypeName string) (*storage.EntityQueryResult, error) {
	client, err := getTableServiceClient()
	if err != nil {
		return nil, err
	}
	table := client.GetTableReference(StateTableName)
	guid := uuid.New().String()
	options := storage.QueryOptions{
		RequestID: guid,
		Filter:    fmt.Sprintf("PartitionKey eq '%s' and ResourceProvider eq '%s' and ResourceType eq '%s'", partitionKey, resourceProviderName, resourceTypeName),
		Select:    []string{"RowKey"},
	}
	return table.QueryEntities(timeout, storage.NoMetadata, &options)
}
func getRowKeyFromResourceId(resourceId string) string {
	return strings.ReplaceAll(resourceId, "/", "!")
}
func GetResourceIdFromRowKey(rowKey string) string {
	return strings.ReplaceAll(rowKey, "!", "/")
}

func PutAsyncOp(partitionKey string, operationId string, action string, status string, result interface{}) error {
	client, err := getTableServiceClient()
	if err != nil {
		return err
	}
	rowkey := operationId
	table := client.GetTableReference(AsyncOperationTableName)
	row := table.GetEntityReference(partitionKey, rowkey)
	p := make(map[string]interface{})
	p["action"] = action
	p["status"] = status
	if result != nil {
		p["output"] = result
	}
	row.Properties = p
	guid := uuid.New().String()
	options := storage.EntityOptions{
		Timeout:   timeout,
		RequestID: guid,
	}
	log.Debugf("Put AsyncOp for partition key: %s operationId: %s id: %s action:%s status %s", partitionKey, operationId, guid, action, status)
	err = row.InsertOrReplace(&options)
	return err
}

func GetAsyncOp(partitionKey string, operationId string) (*AsyncOperationState, error) {
	client, err := getTableServiceClient()
	if err != nil {
		return nil, err
	}
	rowkey := operationId
	table := client.GetTableReference(AsyncOperationTableName)
	row := table.GetEntityReference(partitionKey, rowkey)
	guid := uuid.New().String()
	options := storage.GetEntityOptions{
		RequestID: guid,
	}
	log.Debugf("Get AsyncOp for partition key: %s operationId: %s id: %s", partitionKey, operationId, guid)
	err = row.Get(timeout, storage.MinimalMetadata, &options)
	if err != nil {
		log.Debugf("Failed to GET state for %s", operationId)
		return nil, err
	}
	action := ""
	action, _ = row.Properties["action"].(string)
	status := ""
	status, _ = row.Properties["status"].(string)
	output := ""
	output, _ = row.Properties["output"].(string)

	return &AsyncOperationState{Action: action, Status: status, Output: output}, nil
}
