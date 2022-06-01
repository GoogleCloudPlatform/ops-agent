
function process(tag, timestamp, record)
local __field_0 = (function()
return record["http_request_referer"]
end)();
local __field_1 = (function()
return record["http_request_remoteIp"]
end)();
local __field_2 = (function()
return record["http_request_requestMethod"]
end)();
local __field_3 = (function()
return record["http_request_serverIp"]
end)();
local __field_4 = (function()
return record["http_request_status"]
end)();
local __field_5 = (function()
return record["http_request_userAgent"]
end)();
(function(value)
record["http_request_referer"] = value
end)(nil);
(function(value)
record["http_request_remoteIp"] = value
end)(nil);
(function(value)
record["http_request_requestMethod"] = value
end)(nil);
(function(value)
record["http_request_serverIp"] = value
end)(nil);
(function(value)
record["http_request_status"] = value
end)(nil);
(function(value)
record["http_request_userAgent"] = value
end)(nil);
local v = __field_0;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["referer"] = value
end)(v)
local v = __field_1;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["remoteIp"] = value
end)(v)
local v = __field_2;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["requestMethod"] = value
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
local v = __field_5;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["userAgent"] = value
end)(v)
local v = "agent.googleapis.com/iis_access";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
return 2, timestamp, record
end
