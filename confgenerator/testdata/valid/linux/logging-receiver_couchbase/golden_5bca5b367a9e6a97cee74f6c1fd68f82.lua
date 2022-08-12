
function process(tag, timestamp, record)
local __field_0 = (function()
return record["http_request_protocol"]
end)();
local __field_1 = (function()
return record["http_request_remoteIp"]
end)();
local __field_2 = (function()
return record["http_request_requestMethod"]
end)();
local __field_3 = (function()
return record["http_request_requestUrl"]
end)();
local __field_4 = (function()
return record["http_request_responseSize"]
end)();
local __field_5 = (function()
return record["http_request_status"]
end)();
local __field_6 = (function()
return record["http_request_userAgent"]
end)();
(function(value)
record["http_request_protocol"] = value
end)(nil);
(function(value)
record["http_request_remoteIp"] = value
end)(nil);
(function(value)
record["http_request_requestMethod"] = value
end)(nil);
(function(value)
record["http_request_requestUrl"] = value
end)(nil);
(function(value)
record["http_request_responseSize"] = value
end)(nil);
(function(value)
record["http_request_status"] = value
end)(nil);
(function(value)
record["http_request_userAgent"] = value
end)(nil);
local v = __field_0;
(function(value)
if record["logging.googleapis.com/httpRequest"] == nil
then
record["logging.googleapis.com/httpRequest"] = {}
end
record["logging.googleapis.com/httpRequest"]["protocol"] = value
end)(v)
local v = __field_1;
(function(value)
if record["logging.googleapis.com/httpRequest"] == nil
then
record["logging.googleapis.com/httpRequest"] = {}
end
record["logging.googleapis.com/httpRequest"]["remoteIp"] = value
end)(v)
local v = __field_2;
(function(value)
if record["logging.googleapis.com/httpRequest"] == nil
then
record["logging.googleapis.com/httpRequest"] = {}
end
record["logging.googleapis.com/httpRequest"]["requestMethod"] = value
end)(v)
local v = __field_3;
(function(value)
if record["logging.googleapis.com/httpRequest"] == nil
then
record["logging.googleapis.com/httpRequest"] = {}
end
record["logging.googleapis.com/httpRequest"]["requestUrl"] = value
end)(v)
local v = __field_4;
(function(value)
if record["logging.googleapis.com/httpRequest"] == nil
then
record["logging.googleapis.com/httpRequest"] = {}
end
record["logging.googleapis.com/httpRequest"]["responseSize"] = value
end)(v)
local v = __field_5;
(function(value)
if record["logging.googleapis.com/httpRequest"] == nil
then
record["logging.googleapis.com/httpRequest"] = {}
end
record["logging.googleapis.com/httpRequest"]["status"] = value
end)(v)
local v = __field_6;
(function(value)
if record["logging.googleapis.com/httpRequest"] == nil
then
record["logging.googleapis.com/httpRequest"] = {}
end
record["logging.googleapis.com/httpRequest"]["userAgent"] = value
end)(v)
local v = "agent.googleapis.com/couchbase_http_access";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
return 2, timestamp, record
end
