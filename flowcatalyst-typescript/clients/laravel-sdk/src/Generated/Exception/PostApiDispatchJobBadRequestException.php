<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiDispatchJobBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ErrorResponse4
     */
    private $errorResponse4;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ErrorResponse4 $errorResponse4, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Invalid request - missing or invalid fields');
        $this->errorResponse4 = $errorResponse4;
        $this->response = $response;
    }
    public function getErrorResponse4(): \FlowCatalyst\Generated\Model\ErrorResponse4
    {
        return $this->errorResponse4;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}