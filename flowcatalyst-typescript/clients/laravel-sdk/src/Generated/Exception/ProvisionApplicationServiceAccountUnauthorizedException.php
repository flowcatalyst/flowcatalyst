<?php

namespace FlowCatalyst\Generated\Exception;

class ProvisionApplicationServiceAccountUnauthorizedException extends UnauthorizedException
{
    /**
     * @var \FlowCatalyst\Generated\Model\UnauthorizedResponse
     */
    private $unauthorizedResponse;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\UnauthorizedResponse $unauthorizedResponse, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Not authenticated');
        $this->unauthorizedResponse = $unauthorizedResponse;
        $this->response = $response;
    }
    public function getUnauthorizedResponse(): \FlowCatalyst\Generated\Model\UnauthorizedResponse
    {
        return $this->unauthorizedResponse;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}