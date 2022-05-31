
function process(tag, timestamp, record)
local __field_0 = (function()
return record["http_request_remoteIp"]
end)();
local __field_1 = (function()
return record["http_request_requestMethod"]
end)();
local __field_2 = (function()
return record["http_request_responseSize"]
end)();
local __field_3 = (function()
return record["http_request_serverIp"]
end)();
local __field_4 = (function()
return record["http_request_status"]
end)();
local __field_6 = (function()
return record["level"]
end)();
(function(value)
record["http_request_remoteIp"] = value
end)(nil);
(function(value)
record["http_request_requestMethod"] = value
end)(nil);
(function(value)
record["http_request_responseSize"] = value
end)(nil);
(function(value)
record["http_request_serverIp"] = value
end)(nil);
(function(value)
record["http_request_status"] = value
end)(nil);
local v = __field_0;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["remoteIp"] = value
end)(v)
local v = __field_1;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["requestMethod"] = value
end)(v)
local v = __field_2;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["responseSize"] = value
end)(v)
local v = __field_3;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["serverIp"] = value
end)(v)
local v = __field_4;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["status"] = value
end)(v)
local v = "agent.googleapis.com/couchdb";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_6;
if v == "alert" then v = "ALERT"
elseif v == "crit" then v = "CRITICAL"
elseif v == "critical" then v = "CRITICAL"
elseif v == "debug" then v = "DEBUG"
elseif v == "emerg" then v = "EMERGENCY"
elseif v == "emergency" then v = "EMERGENCY"
elseif v == "err" then v = "ERROR"
elseif v == "error" then v = "ERROR"
elseif v == "info" then v = "INFO"
elseif v == "notice" then v = "NOTICE"
elseif v == "warn" then v = "WARNING"
elseif v == "warning" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
