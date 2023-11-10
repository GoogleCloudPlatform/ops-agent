
function process(tag, timestamp, record)
local match = ((function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("foo")) end)((function()
return record["logging.googleapis.com/trace"]
end)()));

  if match then
    return -1, 0, 0
  end
  return 2, 0, record
end