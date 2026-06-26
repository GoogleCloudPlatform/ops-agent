
function process(tag, timestamp, record)
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
