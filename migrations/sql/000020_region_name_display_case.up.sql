CREATE OR REPLACE FUNCTION api_kurir_region_display_name(input text)
RETURNS text
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
DECLARE
    normalized text;
    result text;
    character text;
    position integer;
    capitalize_next boolean := true;
BEGIN
    normalized := lower(trim(regexp_replace(input, '\s+', ' ', 'g')));
    result := '';

    FOR position IN 1..char_length(normalized) LOOP
        character := substr(normalized, position, 1);
        IF character ~ '[[:alpha:]]' THEN
            IF capitalize_next THEN
                result := result || upper(character);
            ELSE
                result := result || character;
            END IF;
            capitalize_next := false;
        ELSIF character ~ '[[:digit:]]' THEN
            result := result || character;
            capitalize_next := false;
        ELSE
            result := result || character;
            capitalize_next := character NOT IN ('''', '’');
        END IF;
    END LOOP;

    result := regexp_replace(result, '\mDki\M', 'DKI', 'g');
    result := regexp_replace(result, '\mDi\M', 'DI', 'g');
    result := regexp_replace(result, '\mNad\M', 'NAD', 'g');
    result := regexp_replace(result, '\mNtb\M', 'NTB', 'g');
    result := regexp_replace(result, '\mNtt\M', 'NTT', 'g');
    result := regexp_replace(result, '\mViii\M', 'VIII', 'g');
    result := regexp_replace(result, '\mVii\M', 'VII', 'g');
    result := regexp_replace(result, '\mIii\M', 'III', 'g');
    result := regexp_replace(result, '\mVi\M', 'VI', 'g');
    result := regexp_replace(result, '\mIv\M', 'IV', 'g');
    result := regexp_replace(result, '\mIi\M', 'II', 'g');
    result := regexp_replace(result, '\mIx\M', 'IX', 'g');

    RETURN result;
END;
$$;

UPDATE locations
SET province = api_kurir_region_display_name(province),
    city = api_kurir_region_display_name(city),
    district = api_kurir_region_display_name(district),
    subdistrict = api_kurir_region_display_name(subdistrict),
    updated_at = now()
WHERE province IS DISTINCT FROM api_kurir_region_display_name(province)
   OR city IS DISTINCT FROM api_kurir_region_display_name(city)
   OR district IS DISTINCT FROM api_kurir_region_display_name(district)
   OR subdistrict IS DISTINCT FROM api_kurir_region_display_name(subdistrict);

DROP FUNCTION api_kurir_region_display_name(text);
