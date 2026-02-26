<?php

return [
    'openapi-file' => __DIR__ . '/openapi-processed.json',
    'namespace' => 'FlowCatalyst\\Generated',
    'directory' => __DIR__ . '/src/Generated',
    'strict' => false,
    'clean-generated' => true,
    'use-fixer' => true,
    'fixer-config-file' => null,
    'throw-unexpected-status-code' => false,
];
