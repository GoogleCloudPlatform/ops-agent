
function process(tag, timestamp, record)
local __field_0 = (function()
return record["http_request_protocol"]
end)();
local __field_1 = (function()
return record["http_request_referer"]
end)();
local __field_2 = (function()
return record["http_request_remoteIp"]
end)();
local __field_3 = (function()
return record["http_request_requestMethod"]
end)();
local __field_4 = (function()
return record["http_request_requestUrl"]
end)();
local __field_5 = (function()
return record["http_request_responseSize"]
end)();
local __field_6 = (function()
return record["http_request_status"]
end)();
local __field_7 = (function()
return record["http_request_userAgent"]
end)();
local __field_8 = (function()
return record["host"]
end)();
local __field_9 = (function()
return record["user"]
end)();
local omit8 = (string.lower((function()
return record["host"]
end)()) == string.lower("-"));
local omit9 = (string.lower((function()
return record["user"]
end)()) == string.lower("-"));
local omit1 = (string.lower((function()
return record["http_request_%s"]
end)()) == string.lower("-"));
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
(function(value)
record["http_request_protocol"] = value
end)(nil);
local v = __field_0;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["protocol"] = value
end)(v)
local v = __field_1;
if omit1 then v = nil end;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["referer"] = value
end)(v)
local v = __field_2;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["remoteIp"] = value
end)(v)
local v = __field_3;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["requestMethod"] = value
end)(v)
local v = __field_4;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["requestUrl"] = value
end)(v)
local v = __field_5;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["responseSize"] = value
end)(v)
local v = __field_6;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["status"] = value
end)(v)
local v = __field_7;
(function(value)
if record["logging.googleapis.com/http_request"] == nil
then
record["logging.googleapis.com/http_request"] = {}
end
record["logging.googleapis.com/http_request"]["userAgent"] = value
end)(v)
local v = __field_8;
if omit8 then v = nil end;
(function(value)
record["host"] = value
end)(v)
local v = __field_9;
if omit9 then v = nil end;
(function(value)
record["user"] = value
end)(v)
return 2, timestamp, record
end
