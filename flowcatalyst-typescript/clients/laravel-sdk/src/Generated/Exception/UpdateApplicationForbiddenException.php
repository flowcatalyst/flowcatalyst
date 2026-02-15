<?php

namespace FlowCatalyst\Generated\Exception;

class UpdateApplicationForbiddenException extends ForbiddenException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ForbiddenResponse
     */
    private $forbiddenResponse;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ForbiddenResponse $forbiddenResponse, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Insufficient permissions');
        $this->forbiddenResponse = $forbiddenResponse;
        $this->response = $response;
    }
    public function getForbiddenResponse(): \FlowCatalyst\Generated\Model\ForbiddenResponse
    {
        return $this->forbiddenResponse;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}