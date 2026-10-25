<?php

namespace FlowCatalyst\Generated\Exception;

class UpgradeRequiredException extends \RuntimeException implements ClientException
{
    public function __construct(string $message)
    {
        parent::__construct($message, 426);
    }
}