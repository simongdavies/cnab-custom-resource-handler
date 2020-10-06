package azure

import (
	"encoding/json"
	"fmt"
	"strings"

	"get.porter.sh/porter/pkg/porter"
	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/google/uuid"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	log "github.com/sirupsen/logrus"
)

const timeout = 60

var StorageAccountName string
var StorageAccountKey string
var StateTableName string
var AsyncOperationTableName string

type RPState struct {
	*storage.Entity
}

type AsyncOperationState struct {
	*storage.Entity
}

func getTableServiceClient() (*storage.TableServiceClient, error) {
	client, err := storage.NewBasicClient(StorageAccountName, StorageAccountKey)
	if err != nil {
		return nil, fmt.Errorf("Failed to get Table Service Client: %v", err)
	}
	tableService := client.GetTableService()
	return &tableService, nil
}

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
		return nil, fmt.Errorf("error getting state: %v", err)
	}
	properties := models.BundleCommandProperties{
		BundlePullOptions: &porter.BundlePullOptions{},
	}
	err = json.Unmarshal([]byte(row.Properties["Parameters"].(string)), &properties.Parameters)
	if err != nil {
		return nil, fmt.Errorf("Failed to de-serialise parameters: %v", err)
	}
	err = json.Unmarshal([]byte(row.Properties["Credentials"].(string)), &properties.Credentials)
	if err != nil {
		return nil, fmt.Errorf("Failed to de-serialise credentials: %v", err)
	}
	// TODO use reflection
	properties.Tag = row.Properties["Tag"].(string)
	properties.InsecureRegistry = row.Properties["InsecureRegistry"].(bool)
	properties.Force = row.Properties["Force"].(bool)
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
	p["Tag"] = properties.Tag
	p["InsecureRegistry"] = properties.InsecureRegistry
	p["Force"] = properties.Force
	p["RPType"] = RPType
	row.Properties = p
	guid := uuid.New().String()
	options := storage.EntityOptions{
		Timeout:   timeout,
		RequestID: guid,
	}
	log.Debugf("Put RP State for parition key: %s row key: %s id: %s", partitionKey, rowkey, guid)
	err = row.InsertOrReplace(&options)
	return err
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
func ListRPState(partitionKey string) (*storage.EntityQueryResult, error) {
	client, err := getTableServiceClient()
	if err != nil {
		return nil, err
	}
	table := client.GetTableReference(StateTableName)
	guid := uuid.New().String()
	options := storage.QueryOptions{
		RequestID: guid,
		Filter:    fmt.Sprintf("PartitionKey eq '%s' and RPType eq '%s'", partitionKey, RPType),
		Select:    []string{"RowKey"},
	}
	return table.QueryEntities(timeout, storage.NoMetadata, &options)
}
func getRowKeyFromResourceId(resourceId string) string {
	return strings.ReplaceAll(resourceId, "/", "-")
}
