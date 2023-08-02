
function process(tag, timestamp, record)
local match = ((function(v) if v == nil then return false end return (string.find(string.lower(tostring(v)), string.lower("IN"), 1, false) ~= nil) end)((function()
return record["Status"]
end)()));

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end