<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiDispatchProcessUnauthorizedException extends UnauthorizedException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ProcessResponse
     */
    private $processResponse;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ProcessResponse $processResponse, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Invalid or missing authentication token');
        $this->processResponse = $processResponse;
        $this->response = $response;
    }
    public function getProcessResponse(): \FlowCatalyst\Generated\Model\ProcessResponse
    {
        return $this->processResponse;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}