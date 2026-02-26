<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminConfigByAppCodeBySectionForbiddenException extends ForbiddenException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionGetResponse403
     */
    private $apiAdminConfigAppCodeSectionGetResponse403;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionGetResponse403 $apiAdminConfigAppCodeSectionGetResponse403, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAppCodeSectionGetResponse403 = $apiAdminConfigAppCodeSectionGetResponse403;
        $this->response = $response;
    }
    public function getApiAdminConfigAppCodeSectionGetResponse403(): \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionGetResponse403
    {
        return $this->apiAdminConfigAppCodeSectionGetResponse403;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}