package core

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func httpServer() {
	// Serve configuration
	var fileHandler = http.FileServer(http.Dir("../resources"))
	http.Handle("/", fileHandler)
	if err := http.ListenAndServe(":8100", nil); err != nil {
		panic("could not start http server")
	}
}

// Used for testing parametrized objects
var igor_string_parameter string = "the string value"
var igor_int_parameter int = -3

func TestMain(m *testing.M) {

	// Initialize mysql container

	ctx := context.Background()
	_, mysqlPort := SetupMysql(ctx)
	os.Setenv("IGOR_TEST_MYSQL_PORT", fmt.Sprintf("%d", mysqlPort))

	// Initialize additional environment variables
	os.Setenv("IGOR_STR_PARAM", igor_string_parameter)
	os.Setenv("IGOR_INT_PARAM", fmt.Sprintf("%d", igor_int_parameter))

	// Start the server for configuration
	go httpServer()

	// Initialize the Config Objects
	bootFile := "resources/searchRules.json"
	instanceName := "testConfig"

	InitPolicyConfigInstance(bootFile, instanceName, nil, true)
	InitHttpHandlerConfigInstance(bootFile, instanceName, nil, false)

	os.Exit(m.Run())
}

// Mark container as this specific type
type MysqlContainer struct {
	testcontainers.Container
}

// Spin up container for Mysql
func SetupMysql(ctx context.Context) (*MysqlContainer, int) {
	req := testcontainers.ContainerRequest{
		Image:        "mysql:8.0.32",
		ExposedPorts: []string{"3306/tcp", "33060/tcp"},
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": "secret",
			"MYSQL_DATABASE":      "PSBA",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("port: 3306  MySQL Community Server - GPL"),
			wait.ForListeningPort("3306/tcp"),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	if err != nil {
		panic(err)
	}

	mappedPort, _ := container.MappedPort(ctx, "3306/tcp")

	populateMysql(ctx, &MysqlContainer{Container: container}, mappedPort.Int(), schema, data)

	return &MysqlContainer{Container: container}, mappedPort.Int()
}

// Create database, schema and insert test data
func populateMysql(ctx context.Context, container *MysqlContainer, port int, schema string, data string) {

	dbHandle, err := sql.Open("mysql", fmt.Sprintf("root:secret@tcp(localhost:%d)/mysql?parseTime=true&multiStatements=true", port))
	if err != nil {
		panic(err)
	}

	defer dbHandle.Close()

	// Create schema
	_, err = dbHandle.Exec(schema)
	if err != nil {
		panic(err)
	}
	_, err = dbHandle.Exec(data)
	if err != nil {
		panic(err)
	}
}

var schema = `
DROP DATABASE IF EXISTS PSBA;
CREATE DATABASE PSBA;
USE PSBA;

-- PSBA is not a system of record
-- Deleted clients do not exist here
-- You may remove all PoU for a client if resources need to be freed but the client record is needed
-- Campaing management is performed externally. Here, only a mark stating whether the user should be redirected to the
-- captive portal is used (NotificationExpDate)
CREATE TABLE IF NOT EXISTS clients (
    ClientId INT AUTO_INCREMENT PRIMARY KEY,
    ExternalClientId VARCHAR(64) NOT NULL,
    ContractId VARCHAR(64),
    PersonalId VARCHAR(64),
    SecondaryId VARCHAR(64),
    ISP VARCHAR(32),
    BillingCycle INT,
    PlanName VARCHAR(32) NOT NULL,
    BlockingStatus INT NOT NULL,
    PlanOverride VARCHAR(64),
    PlanOverrideExpDate TIMESTAMP,
    AddonProfileOverride VARCHAR(64),
    AddonProfileOverrideExpDate TIMESTAMP,
    NotificationExpDate TIMESTAMP,    -- Client in a campaign will have a not null value
    Parameters JSON, -- Array of {"<parametername>": <parametervalue> [,"expDate": <expiraton date>]}
    Version INT Default 0
);


CREATE UNIQUE INDEX ClientsExternalClientId_idx ON clients (ExternalClientId);
CREATE INDEX ClientsContractId_idx ON clients (ContractId);
CREATE INDEX ClientsPersonalId_idx ON clients (PersonalId);

-- Definition of allowed Parameters for clients table. Used in the Parameters field. Valid parameter names 
-- or types are enforced by the API
CREATE TABLE IF NOT EXISTS clientParametersDef (
    parameterName VARCHAR(64) NOT NULL PRIMARY KEY,
    description VARCHAR(200),
    type INT NOT NULL       -- 0: String, 1: Integer: 2: Date
);

-- Access line for fixed network
CREATE TABLE IF NOT EXISTS pou (
    PoUId INT AUTO_INCREMENT PRIMARY KEY,
    ClientId INT REFERENCES Clients(ClientId),
    AccessPort INT,           -- Typically, a NAS-Port
    AccessId VARCHAR(128),    -- May be an CircuitId, or RemoteId, BNG group or BNG Address to be used in combination with NAS-Port
    UserName VARCHAR(128),
    Password VARCHAR(128),    -- Password may be stored in clear or with {algorithm}<value>
    IPv4Address VARCHAR(32),
    IPv6DelegatedPrefix VARCHAR(64),
    IPv6WANPrefix VARCHAR(64),
    AccessType INT,
    CheckType INT,             -- 0: Use line data only. 1: Check line and userName
    Version INT Default 0
);

CREATE INDEX PouClient_idx ON pou (ClientId);
CREATE INDEX PouAccessIdPort_idx ON pou (AccessId, AccessPort);
CREATE INDEX PoUUserName_idx ON pou (UserName);
CREATE INDEX PoUIPv4Address_idx ON pou (IPv4Address);

CREATE TABLE IF NOT EXISTS planProfiles (
    PlanName VARCHAR(64) PRIMARY KEY,
    ExternalPlanName VARCHAR(128),
    ProfileName VARCHAR(64)
);

-- To be replaced in plan profiles. This way, a single profile
-- may exist for all basic services, the speed being a parameter
CREATE TABLE IF NOT EXISTS planParameters (
    PlanName VARCHAR(64) PRIMARY KEY REFERENCES PlanProfiles(PlanName),
    Parameters JSON
);

-- Typically, radius clients. The IP address of the node is one of the parameters
CREATE TABLE IF NOT EXISTS accessNodes (
    AccessNodeId VARCHAR(64) PRIMARY KEY,
    Parameters JSON
);

-- Just used for validation
CREATE TABLE IF NOT EXISTS addonProfiles (
    ProfileName VARCHAR(64) PRIMARY KEY
);

-- Admin tables
CREATE TABLE IF NOT EXISTS operators (
    OperatorId INT AUTO_INCREMENT PRIMARY KEY,
    OperatorName VARCHAR(64) NOT NULL,
    Passwd VARCHAR(64),
    IsDisabled INT NOT NULL
);

CREATE INDEX Operator_idx ON operators(operatorName);

CREATE TABLE IF NOT EXISTS roles (
    role VARCHAR(64) PRIMARY KEY NOT NULL,
    description VARCHAR(200)
);

CREATE TABLE IF NOT EXISTS rolepermissions (
    Role VARCHAR(64) REFERENCES roles(role),
    Path VARCHAR(128) NOT NULL,
    Method VARCHAR(10) NOT NULL,
    PRIMARY KEY (role, path, method)
);

CREATE TABLE IF NOT EXISTS operatorroles (
    OperatorId INT REFERENCES operators(operatorId),
    Role VARCHAR(64) NOT NULL,
    PRIMARY KEY (operatorId, role)
);

CREATE TABLE IF NOT EXISTS audit_log (
    Id INT AUTO_INCREMENT PRIMARY KEY,
    OperatorName VARCHAR(64) NOT NULL,
    Date TIMESTAMP NOT NULL,
    ClientId INT,
    ExternalClientId VARCHAR(64),
    OperationType VARCHAR(64),
    InitialState JSON,  -- JSON with object being modified or null if created
    FinalState JSON,    -- JSON with final state of the object
    Method VARCHAR(10),         -- POST, PUT or PATCH
    ResultCode INT              -- HTTP Status code
);

create user if not exists 'psbauser' identified by 'psbasecret';
grant all privileges on *.* to 'psbauser';
`

var data = `
-- No username or password
INSERT INTO clients (ExternalClientId, PlanName, BlockingStatus, NotificationExpDate, Parameters) values ('External1', 'Plan1', 0, '2022-01-01 00:00:00', '[{"mirror": true, "expDate": "2022-07-09T14:30:23 UTC"}]');
INSERT INTO pou (ClientId, AccessPort, AccessId) values (LAST_INSERT_ID(), 1, "127.0.0.1");

-- Only password
INSERT INTO clients (ExternalClientId, PlanName, BlockingStatus) values ('External2', 'Plan1', 0);
INSERT INTO pou (ClientId, AccessPort, AccessId, password) values (LAST_INSERT_ID(), 2, "127.0.0.1", "francisco");

-- Only username
INSERT INTO clients (ExternalClientId, PlanName, BlockingStatus) values ('External3', 'Plan1', 0);
INSERT INTO pou (ClientId, AccessPort, AccessId, UserName) values (LAST_INSERT_ID(), 3, "127.0.0.1", "francisco@indra.es");

-- Username and password
INSERT INTO clients (ExternalClientId, PlanName, BlockingStatus) values ('External4', 'Plan1', 0);
INSERT INTO pou (ClientId, AccessPort, AccessId, userName, password) values (LAST_INSERT_ID(), 4, "127.0.0.1", "francisco@database.provision.nopermissive.doreject.block_addon.proxy", "francisco");

-- Blocked
INSERT INTO clients (ExternalClientId, PlanName, BlockingStatus) values ('External5', 'Plan1', 2);
INSERT INTO pou (ClientId, AccessPort, AccessId) values (LAST_INSERT_ID(), 5, "127.0.0.1");

-- Notification
INSERT INTO clients (ExternalClientId, PlanName, BlockingStatus, NotificationExpDate) values ('External6', 'Plan1', 0, '2025-01-01 00:00:00');
INSERT INTO pou (ClientId, AccessPort, AccessId) values (LAST_INSERT_ID(), 6, "127.0.0.1");

-- PlanOverride
INSERT INTO clients (ExternalClientId, PlanName, BlockingStatus, PlanOverride, PlanOverrideExpDate) values ('External7', 'Plan1', 0, 'Plan2', '2025-01-01 00:00:00');
INSERT INTO pou (ClientId, AccessPort, AccessId) values (LAST_INSERT_ID(), 7, "127.0.0.1");

-- AddonOverride
INSERT INTO clients (ExternalClientId, PlanName, BlockingStatus, AddonProfileOverride, AddonProfileOverrideExpDate) values ('External8', 'Plan1', 0, 'vala', '2025-01-01 00:00:00');
INSERT INTO pou (ClientId, AccessPort, AccessId) values (LAST_INSERT_ID(), 8, "127.0.0.1");

INSERT INTO planProfiles (PlanName, ExternalPlanName, ProfileName) values ('Plan1', 'ExtPlan1', "Prof1");
INSERT INTO planProfiles (PlanName, ExternalPlanName, ProfileName) values ('Plan2', 'ExtPlan2', "Prof2");
INSERT INTO planProfiles (PlanName, ExternalPlanName, ProfileName) values ('PlanOverriden', 'ExtPlanOverriden', "Prof0");

-- Radius Clients
INSERT INTO accessNodes (AccessNodeId, Parameters) values ("127.0.0.1", '{"name": "RepublicaHW01", "ipAddress": "127.0.0.1", "secret": "mysecret", "clientClass": "Huawei", "attributes": [{"Redback-Primary-DNS": "1.2.3.4"}, {"Session-Timeout": 3600}]}');
INSERT INTO accessNodes (AccessNodeId, Parameters) values ("127.0.0.2", '{"name": "RepublicaHW02", "ipAddress": "127.0.0.2", "secret": "mysecret", "clientClass": "Huawei", "attributes": [{"Redback-Primary-DNS": "1.2.3.4"}, {"Session-Timeout": 7200}]}');

-- Plan parameters
INSERT INTO planParameters (PlanName, Parameters) values ("Plan1", '{"Speed": 1000, "Message": "hello plan 1"}');
INSERT INTO planParameters (PlanName, Parameters) values ("Plan2", '{"Speed": 2000, "Message": "hello plan 2"}');
`
