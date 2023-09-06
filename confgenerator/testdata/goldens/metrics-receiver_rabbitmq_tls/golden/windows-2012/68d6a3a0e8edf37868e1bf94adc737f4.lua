
function process(tag, timestamp, record)
local __field_0 = (function()
return record["message"]
end)();
local v = __field_0;
if v == "LogParseErr" then v = "Ops Agent failed to parse logs, Code: LogParseErr, Documentation: https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/troubleshoot-find-info"
elseif v == "LogPipelineErr" then v = "Ops Agent logging pipeline failed, Code: LogPipelineErr, Documentation: https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/troubleshoot-find-info"
end
(function(value)
record["message"] = value
end)(v)
local v = "ops-agent";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["agent.googleapis.com/health/agentKind"] = value
end)(v)
local v = "latest";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["agent.googleapis.com/health/agentVersion"] = value
end)(v)
local v = "v1";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["agent.googleapis.com/health/schemaVersion"] = value
end)(v)
return 2, timestamp, record
end
