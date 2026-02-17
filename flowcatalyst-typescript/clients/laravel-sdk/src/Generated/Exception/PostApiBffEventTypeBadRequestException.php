<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypeBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse400
     */
    private $apiBffEventTypesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse400 $apiBffEventTypesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesPostResponse400 = $apiBffEventTypesPostResponse400;
        $this->response = $response;
    }
    public function getApiBffEventTypesPostResponse400(): \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse400
    {
        return $this->apiBffEventTypesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}