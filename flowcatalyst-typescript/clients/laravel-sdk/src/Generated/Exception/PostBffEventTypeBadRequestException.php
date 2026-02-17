<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypeBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesPostResponse400
     */
    private $bffEventTypesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesPostResponse400 $bffEventTypesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesPostResponse400 = $bffEventTypesPostResponse400;
        $this->response = $response;
    }
    public function getBffEventTypesPostResponse400(): \FlowCatalyst\Generated\Model\BffEventTypesPostResponse400
    {
        return $this->bffEventTypesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}