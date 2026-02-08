<?php

/**
 * Pre-processes the OpenAPI JSON spec for JanePHP compatibility.
 *
 * Fixes:
 * - allOf with $ref + default in query parameters → simplified to just the $ref
 * - Empty schemas {} → replaced with { "type": "object" }
 */

$inputFile = $argv[1] ?? __DIR__ . '/../../../../core/flowcatalyst-platform/openapi/openapi.json';
$outputFile = $argv[2] ?? __DIR__ . '/../openapi-processed.json';

$json = json_decode(file_get_contents($inputFile), true);
if ($json === null) {
    fwrite(STDERR, "Failed to parse JSON from: {$inputFile}\n");
    exit(1);
}

$fixed = 0;

foreach ($json['paths'] as $path => &$methods) {
    foreach ($methods as $method => &$operation) {
        if (!is_array($operation)) continue;

        // Fix parameters with allOf[$ref, {default}]
        if (isset($operation['parameters'])) {
            foreach ($operation['parameters'] as $i => &$param) {
                if (isset($param['schema']['allOf'])) {
                    $allOf = $param['schema']['allOf'];
                    if (count($allOf) === 2 && isset($allOf[0]['$ref']) && isset($allOf[1]['default'])) {
                        $json['paths'][$path][$method]['parameters'][$i]['schema'] = $allOf[0];
                        $fixed++;
                    }
                }
            }
            unset($param);
        }

        // Fix empty response schemas
        if (isset($operation['responses'])) {
            foreach ($operation['responses'] as $code => &$response) {
                if (isset($response['content'])) {
                    foreach ($response['content'] as $mediaType => &$content) {
                        if (isset($content['schema']) && ($content['schema'] === [] || empty($content['schema']))) {
                            $json['paths'][$path][$method]['responses'][$code]['content'][$mediaType]['schema'] = ['type' => 'object'];
                            $fixed++;
                        }
                    }
                    unset($content);
                }
            }
            unset($response);
        }
    }
    unset($operation);
}
unset($methods);

file_put_contents($outputFile, json_encode($json, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES));
echo "Processed OpenAPI spec: {$fixed} fixes applied. Output: {$outputFile}\n";
