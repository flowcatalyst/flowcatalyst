<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypeConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesPostResponse409
     */
    private $bffEventTypesPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesPostResponse409 $bffEventTypesPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesPostResponse409 = $bffEventTypesPostResponse409;
        $this->response = $response;
    }
    public function getBffEventTypesPostResponse409(): \FlowCatalyst\Generated\Model\BffEventTypesPostResponse409
    {
        return $this->bffEventTypesPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}