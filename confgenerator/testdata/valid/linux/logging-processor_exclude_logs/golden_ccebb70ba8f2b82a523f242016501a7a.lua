
function process(tag, timestamp, record)
local match = ((string.lower((function()
return record["\226\152\131"]
end)()) == string.lower("foo")))

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end