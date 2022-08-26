
function process(tag, timestamp, record)
local match = (((((function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("foo")) end)((function()
return record["message"]
end)()) and (record["__match_0_0_0_0_1"] ~= nil)) or (not (record["__match_0_0_0_1"] ~= nil))) and (function(v) if v == nil then return false end return (string.find(string.lower(tostring(v)), string.lower("frob"), 1, false) != nil) end)((function()
return record["message"]
end)())));

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