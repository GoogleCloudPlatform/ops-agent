
	function iis_concat_fields(tag, timestamp, record)
		if (record["cs_uri_query"] == "-") then
	    record["cs_uri_query"] = nil
	  end
	  if (record["http_request_referer"] == "-") then
	    record["http_request_referer"] = nil
	  end
	  if (record["user"] == "-") then
	    record["user"] = nil
	  end
		
	  -- Concatenate serverIp with port
	  if record["http_request_serverIp"] ~= nil and record["s_port"] ~= nil then
		record["http_request_serverIp"] = table.concat({record["http_request_serverIp"], ":", record["s_port"]})
	  end
	  
	  -- Build requestUrl from stem and query
	  if record["cs_uri_stem"] ~= nil then
		if record["cs_uri_query"] == nil or record["cs_uri_query"] == "" or record["cs_uri_query"] == "-" then
		  record["http_request_requestUrl"] = record["cs_uri_stem"]
		else
		  record["http_request_requestUrl"] = table.concat({record["cs_uri_stem"], "?", record["cs_uri_query"]})
		end
	  end
	  
	  -- Clean up intermediate fields
	  record["cs_uri_stem"] = nil
	  record["cs_uri_query"] = nil
	  record["s_port"] = nil
	  
	  return 2, timestamp, record
	end
	