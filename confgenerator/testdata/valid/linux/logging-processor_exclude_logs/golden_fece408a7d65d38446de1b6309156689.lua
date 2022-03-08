
function process(tag, timestamp, record)
local match = (((string.lower((function()
return record["message"]
end)()) == string.lower("foo")) or (string.lower((function()
return record["message"]
end)()) ~= string.lower("bar")) or (string.find(string.lower((function()
return record["message"]
end)()), string.lower("baz"), 1, false) != nil) or (record["__match_0_0_3"] != nil) or (record["__match_0_0_4"] != nil)))

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