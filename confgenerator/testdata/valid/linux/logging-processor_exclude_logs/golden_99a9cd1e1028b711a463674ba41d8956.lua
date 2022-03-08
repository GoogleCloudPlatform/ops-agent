
function process(tag, timestamp, record)
local match = (((((string.lower((function()
return record["message"]
end)()) == string.lower("foo")) and (record["__match_0_0_0_0_1"] != nil)) or (record["__match_0_0_0_1"] != nil)) and (string.find(string.lower((function()
return record["message"]
end)()), string.lower("frob"), 1, false) != nil)))

for k, v in pairs(record) do
  if string.match(k, "^__match_.+") then
    record[k] = nil
  end
end

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end