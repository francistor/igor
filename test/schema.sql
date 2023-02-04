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
