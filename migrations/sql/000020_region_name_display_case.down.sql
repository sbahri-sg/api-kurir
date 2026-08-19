UPDATE locations
SET province = upper(province),
    city = upper(city),
    district = upper(district),
    subdistrict = upper(subdistrict),
    updated_at = now()
WHERE province IS DISTINCT FROM upper(province)
   OR city IS DISTINCT FROM upper(city)
   OR district IS DISTINCT FROM upper(district)
   OR subdistrict IS DISTINCT FROM upper(subdistrict);
