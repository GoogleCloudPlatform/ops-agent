
function process(tag, timestamp, record)
local match = ((function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("bar")) end)((function()
return record["message"]
end)()));

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end