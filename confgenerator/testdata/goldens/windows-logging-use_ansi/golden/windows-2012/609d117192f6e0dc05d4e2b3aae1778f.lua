
function process(tag, timestamp, record)
local __field_0 = (function()
return record["message"]
end)();
local v = __field_0;
if v == "LogParseErr" then v = "Ops Agent failed to parse logs, Code: LogParseErr, Documentation: https://cloud.google.com/logging/docs/agent/ops-agent/troubleshoot-find-info#health-checks"
elseif v == "LogPipelineErr" then v = "Ops Agent logging pipeline failed, Code: LogPipelineErr, Documentation: https://cloud.google.com/logging/docs/agent/ops-agent/troubleshoot-find-info#health-checks"
end
(function(value)
record["message"] = value
end)(v)
return 2, timestamp, record
end
