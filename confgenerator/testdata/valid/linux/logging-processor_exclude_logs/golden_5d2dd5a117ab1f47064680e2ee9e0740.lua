
function process(tag, timestamp, record)
local match = ((string.lower((function()
return record["message"]
end)()) == string.lower("bar")))

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end