<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypeBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse400
     */
    private $apiAdminEventTypesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse400 $apiAdminEventTypesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesPostResponse400 = $apiAdminEventTypesPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminEventTypesPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse400
    {
        return $this->apiAdminEventTypesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}