
function process(tag, timestamp, record)
local v = "test-cluster";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/attributes/dataproc-cluster-name"] = value
end)(v)
local v = "test-uuid";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/attributes/dataproc-cluster-uuid"] = value
end)(v)
local v = "test-region";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/attributes/dataproc-region"] = value
end)(v)
return 2, timestamp, record
end
