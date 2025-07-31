
function process(tag, timestamp, record)
local __field_0 = (function()
return record["cs_uri_query"]
end)();
local __field_1 = (function()
return record["http_request_referer"]
end)();
local __field_2 = (function()
return record["cs_uri_stem"]
end)();
local __field_3 = (function()
return record["http_request_serverIp"]
end)();
local __field_4 = (function()
return record["user"]
end)();
local omit0 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-")) end)((function()
return record["cs_uri_query"]
end)());
local omit1 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-")) end)((function()
return record["http_request_referer"]
end)());
local omit4 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-")) end)((function()
return record["user"]
end)());
local v = __field_0;
if omit0 then v = nil end;
(function(value)
record["cs_uri_query"] = value
end)(v)
local v = __field_1;
if omit1 then v = nil end;
(function(value)
record["http_request_referer"] = value
end)(v)
local v = __field_2;

			-- Build URL from stem and query
			local stem = __field_2
			local query = record["cs_uri_query"]
			
			-- Handle the case where query is "-" (IIS placeholder for empty)
			if query == "-" then
				query = nil
			end
			
			if stem == nil then
				v = nil
			elseif query == nil or query == "" then
				v = stem
			else
				v = stem .. "?" .. query
			end
			-- Clean up intermediate fields for Fluent Bit
			record["cs_uri_stem"] = nil
			record["cs_uri_query"] = nil
(function(value)
record["http_request_requestUrl"] = value
end)(v)
local v = __field_3;

			-- Concatenate serverIp with port
			local serverIp = __field_3
			local port = record["s_port"]
			if serverIp ~= nil and port ~= nil then
				v = serverIp .. ":" .. port
			end
			-- Clean up intermediate field for Fluent Bit
			record["s_port"] = nil
(function(value)
record["http_request_serverIp"] = value
end)(v)
local v = __field_4;
if omit4 then v = nil end;
(function(value)
record["user"] = value
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
