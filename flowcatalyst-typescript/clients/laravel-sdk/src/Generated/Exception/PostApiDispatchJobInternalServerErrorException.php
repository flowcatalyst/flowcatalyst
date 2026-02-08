<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiDispatchJobInternalServerErrorException extends InternalServerErrorException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ErrorResponse3
     */
    private $errorResponse3;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ErrorResponse3 $errorResponse3, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Internal server error');
        $this->errorResponse3 = $errorResponse3;
        $this->response = $response;
    }
    public function getErrorResponse3(): \FlowCatalyst\Generated\Model\ErrorResponse3
    {
        return $this->errorResponse3;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}