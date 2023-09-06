
function process(tag, timestamp, record)
local match = ((function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("a,:=<>+~\"\\.*\7\8\12\10\13\9\11!!!!b")) end)((function()
return record["message"]
end)()));

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end