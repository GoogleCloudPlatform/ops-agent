
function process(tag, timestamp, record)
local match = ((string.lower((function()
return record["message"]
end)()) == string.lower("a,:=<>+~"\.*\7\8\12\10\13\9\11!!!!b")))

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end