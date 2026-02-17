<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypeConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse409
     */
    private $apiBffEventTypesPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse409 $apiBffEventTypesPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesPostResponse409 = $apiBffEventTypesPostResponse409;
        $this->response = $response;
    }
    public function getApiBffEventTypesPostResponse409(): \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse409
    {
        return $this->apiBffEventTypesPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}