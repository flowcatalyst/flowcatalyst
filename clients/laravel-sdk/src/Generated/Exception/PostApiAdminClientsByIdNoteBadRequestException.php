<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdNoteBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse400
     */
    private $apiAdminClientsIdNotesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse400 $apiAdminClientsIdNotesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdNotesPostResponse400 = $apiAdminClientsIdNotesPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminClientsIdNotesPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse400
    {
        return $this->apiAdminClientsIdNotesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}