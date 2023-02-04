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